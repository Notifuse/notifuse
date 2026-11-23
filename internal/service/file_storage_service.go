package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

//go:generate mockgen -destination=./mocks/mock_object_store.go -package=mocks github.com/Notifuse/notifuse/internal/service ObjectStore

// ObjectStore is the part of an S3 client the file storage service uses.
type ObjectStore interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, expires time.Duration) (*v4.PresignedHTTPRequest, error)
}

// ObjectStoreFactory builds an ObjectStore for a workspace's file storage settings.
type ObjectStoreFactory func(settings domain.FileManagerSettings) (ObjectStore, error)

// NewS3ObjectStoreFactory returns a factory of AWS SDK clients, for any S3-compatible
// provider, that send their requests through httpClient. The app passes an SSRF-protected
// client unless FILE_STORAGE_ALLOW_PRIVATE_HOSTS is set; nil means the SDK's own client.
func NewS3ObjectStoreFactory(httpClient *http.Client) ObjectStoreFactory {
	return func(settings domain.FileManagerSettings) (ObjectStore, error) {
		return newS3ObjectStore(settings, httpClient)
	}
}

// newS3ObjectStore returns a client for the settings' endpoint, region and addressing style.
//
// Checksums are computed only when an operation requires them: since service/s3 v1.73.0
// the SDK adds them by default, and several S3-compatible providers refuse the headers.
func newS3ObjectStore(settings domain.FileManagerSettings, httpClient *http.Client) (ObjectStore, error) {
	if err := domain.ValidateStorageEndpoint(settings.Endpoint); err != nil {
		return nil, err
	}
	region := "us-east-1"
	if settings.Region != nil && *settings.Region != "" {
		region = *settings.Region
	}
	options := s3.Options{
		BaseEndpoint:               aws.String(settings.Endpoint),
		Region:                     region,
		UsePathStyle:               settings.ForcePathStyle,
		Credentials:                credentials.NewStaticCredentialsProvider(settings.AccessKey, settings.SecretKey, ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	if httpClient != nil {
		options.HTTPClient = httpClient
	}
	options.APIOptions = append(options.APIOptions, unsignAcceptEncoding)
	client := s3.New(options)
	return &s3ObjectStore{Client: client, presigner: s3.NewPresignClient(client)}, nil
}

// unsignAcceptEncoding drops Accept-Encoding before the request is signed. Google Cloud
// Storage's S3 interface changes that header on the way in, so a signature that covers it
// never matches there (SignatureDoesNotMatch). No provider needs it signed, and the HTTP
// client sets its own after signing.
func unsignAcceptEncoding(stack *middleware.Stack) error {
	// A presign builds its own stack, which signs the query and has no Signing step.
	if _, ok := stack.Finalize.Get("Signing"); !ok {
		return nil
	}
	return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("UnsignAcceptEncoding",
		func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
			if req, ok := in.Request.(*smithyhttp.Request); ok {
				req.Header.Del("Accept-Encoding")
			}
			return next.HandleFinalize(ctx, in)
		}), "Signing", middleware.Before)
}

type s3ObjectStore struct {
	*s3.Client
	presigner *s3.PresignClient
}

func (s *s3ObjectStore) PresignPutObject(ctx context.Context, params *s3.PutObjectInput, expires time.Duration) (*v4.PresignedHTTPRequest, error) {
	return s.presigner.PresignPutObject(ctx, params, s3.WithPresignExpires(expires))
}

const (
	fileStorageCallTimeout         = 30 * time.Second
	fileStorageListTimeout         = 60 * time.Second
	fileStorageMaxListPages        = 100
	fileStorageDeleteTimeout       = 2 * time.Minute
	fileStorageDeleteFolderTimeout = 5 * time.Minute
	fileStorageDeleteConcurrency   = 8
)

// FileStorageService implements domain.FileStorageService on the workspace's bucket.
type FileStorageService struct {
	authService   domain.AuthService
	workspaceRepo domain.WorkspaceRepository
	stores        ObjectStoreFactory
	now           func() time.Time
}

// NewFileStorageService creates the file storage service.
func NewFileStorageService(authService domain.AuthService, workspaceRepo domain.WorkspaceRepository, stores ObjectStoreFactory) *FileStorageService {
	return &FileStorageService{
		authService:   authService,
		workspaceRepo: workspaceRepo,
		stores:        stores,
		now:           time.Now,
	}
}

// open authenticates the caller, checks the file rights the operation needs, and returns
// a client for the workspace's bucket with the bucket name.
func (s *FileStorageService) open(ctx context.Context, workspaceID string, write bool) (context.Context, ObjectStore, string, error) {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return ctx, nil, "", err
	}
	if write && !userWorkspace.CanWriteFiles() {
		return ctx, nil, "", &domain.ErrUnauthorized{Message: "uploading or deleting files requires write access to templates or the blog"}
	}
	if !write && !userWorkspace.CanReadFiles() {
		return ctx, nil, "", &domain.ErrUnauthorized{Message: "listing files requires access to templates or the blog"}
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return ctx, nil, "", err
	}
	settings := workspace.Settings.FileManager
	if settings.Endpoint == "" || settings.Bucket == "" {
		return ctx, nil, "", domain.NewValidationError("no file storage is configured for this workspace")
	}
	store, err := s.stores(settings)
	if err != nil {
		return ctx, nil, "", err
	}
	return ctx, store, settings.Bucket, nil
}

// storageError turns a provider failure into an error the caller can read; a cancelled or
// timed-out context is reported as such.
func storageError(err error) error {
	if errors.Is(err, safehttpclient.ErrPrivateIP) {
		return &domain.FileStorageError{Message: "the endpoint resolves to a private address, which this server does not reach unless FILE_STORAGE_ALLOW_PRIVATE_HOSTS is set"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &domain.FileStorageError{Message: "the storage did not answer in time"}
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return &domain.FileStorageError{Message: apiErr.ErrorCode() + ": " + apiErr.ErrorMessage()}
	}
	return &domain.FileStorageError{Message: err.Error()}
}

// listAll reads the objects under prefix, following continuation tokens until the last
// page, maxObjects objects, fileStorageMaxListPages pages or fileStorageListTimeout.
func (s *FileStorageService) listAll(ctx context.Context, store ObjectStore, bucket, prefix string, maxObjects int) (*domain.StoredObjectList, error) {
	ctx, cancel := context.WithTimeout(ctx, fileStorageListTimeout)
	defer cancel()

	result := &domain.StoredObjectList{Objects: []domain.StoredObject{}}
	var token *string
	for page := 0; ; page++ {
		if page == fileStorageMaxListPages {
			result.CutShort = true
			return result, nil
		}
		input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), ContinuationToken: token}
		if prefix != "" {
			input.Prefix = aws.String(prefix)
		}
		callCtx, cancelCall := context.WithTimeout(ctx, fileStorageCallTimeout)
		output, err := store.ListObjectsV2(callCtx, input)
		cancelCall()
		if err != nil {
			if page > 0 && ctx.Err() != nil {
				// The overall budget ran out after some pages: say the listing is partial.
				result.CutShort = true
				return result, nil
			}
			return nil, storageError(err)
		}
		for _, object := range output.Contents {
			if len(result.Objects) >= maxObjects {
				result.Capped = true
				return result, nil
			}
			result.Objects = append(result.Objects, domain.StoredObject{
				Key:          aws.ToString(object.Key),
				Size:         aws.ToInt64(object.Size),
				LastModified: aws.ToTime(object.LastModified),
			})
		}
		if !aws.ToBool(output.IsTruncated) {
			return result, nil
		}
		// A provider that says there is more without a token would loop forever.
		if aws.ToString(output.NextContinuationToken) == "" {
			result.CutShort = true
			return result, nil
		}
		if len(result.Objects) >= maxObjects {
			result.Capped = true
			return result, nil
		}
		token = output.NextContinuationToken
	}
}

// List returns the objects of the bucket, under prefix when one is given.
func (s *FileStorageService) List(ctx context.Context, workspaceID, prefix string) (*domain.StoredObjectList, error) {
	if prefix != "" {
		if err := domain.ValidateStorageKeyForDelete(prefix); err != nil {
			return nil, err
		}
	}
	ctx, store, bucket, err := s.open(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	return s.listAll(ctx, store, bucket, prefix, domain.MaxListedStorageObjects)
}

// PresignUpload signs a PUT of one file. Only Content-Type is signed besides the host: the
// browser cannot set Content-Length itself, and providers treat a signed one differently.
func (s *FileStorageService) PresignUpload(ctx context.Context, req domain.PresignUploadRequest) (*domain.PresignedUpload, error) {
	if err := domain.ValidateStorageKey(req.Key); err != nil {
		return nil, err
	}
	if strings.HasSuffix(req.Key, "/") {
		return nil, domain.NewValidationError("a file key cannot end with /")
	}
	contentType, err := domain.NormalizeStorageContentType(req.ContentType)
	if err != nil {
		return nil, err
	}
	ctx, store, bucket, err := s.open(ctx, req.WorkspaceID, true)
	if err != nil {
		return nil, err
	}

	signed, err := store.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(req.Key),
		ContentType: aws.String(contentType),
	}, domain.StorageUploadURLExpiry)
	if err != nil {
		return nil, storageError(err)
	}
	return &domain.PresignedUpload{
		URL:       signed.URL,
		Headers:   map[string]string{"Content-Type": contentType},
		ExpiresAt: s.now().Add(domain.StorageUploadURLExpiry),
	}, nil
}

// CreateFolder writes a folder marker, unless something already lives under the prefix.
func (s *FileStorageService) CreateFolder(ctx context.Context, workspaceID, key string) (*domain.CreateFolderResult, error) {
	if err := domain.ValidateStorageFolderPrefix(key); err != nil {
		return nil, err
	}
	ctx, store, bucket, err := s.open(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, fileStorageCallTimeout)
	existing, err := store.ListObjectsV2(callCtx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(key),
		MaxKeys: aws.Int32(1),
	})
	cancel()
	if err != nil {
		return nil, storageError(err)
	}
	if len(existing.Contents) > 0 {
		return &domain.CreateFolderResult{Status: domain.FolderExists}, nil
	}

	callCtx, cancel = context.WithTimeout(ctx, fileStorageCallTimeout)
	defer cancel()
	if _, err := store.PutObject(callCtx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(nil),
	}); err != nil {
		return nil, storageError(err)
	}
	return &domain.CreateFolderResult{Status: domain.FolderCreated}, nil
}

// deleteOne deletes one key within the call timeout.
func deleteOne(ctx context.Context, store ObjectStore, bucket, key string) error {
	ctx, cancel := context.WithTimeout(ctx, fileStorageCallTimeout)
	defer cancel()
	_, err := store.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

// deleteKeys deletes keys fileStorageDeleteConcurrency at a time and reports which went.
// A key not tried before ctx ends counts as not deleted.
func deleteKeys(ctx context.Context, store ObjectStore, bucket string, keys []string) []bool {
	deleted := make([]bool, len(keys))
	slots := make(chan struct{}, fileStorageDeleteConcurrency)
	var wg sync.WaitGroup
	for i, key := range keys {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			defer func() { <-slots }()
			deleted[i] = ctx.Err() == nil && deleteOne(ctx, store, bucket, key) == nil
		}(i, key)
	}
	wg.Wait()
	return deleted
}

// deletionContext runs deletes to the end of their own budget: a client that disconnects,
// or a proxy that times the request out, must not leave a folder half deleted.
func deletionContext(ctx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), budget)
}

// Delete deletes keys one at a time (several S3-compatible providers have no batch delete),
// counting the failures instead of stopping at the first.
func (s *FileStorageService) Delete(ctx context.Context, workspaceID string, keys []string) (*domain.DeleteFilesResult, error) {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if err := domain.ValidateStorageKeyForDelete(key); err != nil {
			return nil, err
		}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			unique = append(unique, key)
		}
	}
	if len(unique) == 0 {
		return nil, domain.NewValidationError("no key to delete")
	}
	if len(unique) > domain.MaxDeletedStorageKeys {
		return nil, domain.NewValidationError("too many keys: at most 1000 per call")
	}
	ctx, store, bucket, err := s.open(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}

	ctx, cancel := deletionContext(ctx, fileStorageDeleteTimeout)
	defer cancel()
	result := &domain.DeleteFilesResult{}
	for _, ok := range deleteKeys(ctx, store, bucket, unique) {
		if ok {
			result.Deleted++
		} else {
			result.Failed++
		}
	}
	return result, nil
}

// DeleteFolder deletes everything under a folder prefix, its marker included. Files go
// first, then the folder markers from the deepest up: some providers keep a folder that
// still holds something and report success.
func (s *FileStorageService) DeleteFolder(ctx context.Context, workspaceID, prefix string) (*domain.DeleteFolderResult, error) {
	if err := domain.ValidateStorageFolderPrefix(prefix); err != nil {
		return nil, err
	}
	ctx, store, bucket, err := s.open(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}

	listing, err := s.listAll(ctx, store, bucket, prefix, domain.MaxListedStorageObjects)
	if err != nil {
		return nil, err
	}
	// Deleting part of the folder and reporting success would leave files behind unseen.
	if listing.CutShort {
		return nil, &domain.FileStorageError{Message: "the storage did not return the whole content of " + prefix}
	}
	if listing.Capped {
		return nil, domain.NewValidationError("the folder holds more than 10000 objects; delete its subfolders first")
	}

	var files, markers []string
	for _, object := range listing.Objects {
		// A provider that ignored Prefix must not make this delete objects outside the folder.
		if !strings.HasPrefix(object.Key, prefix) {
			continue
		}
		if strings.HasSuffix(object.Key, "/") {
			markers = append(markers, object.Key)
		} else {
			files = append(files, object.Key)
		}
	}
	// The deepest folders first, each once its content is gone.
	sort.SliceStable(markers, func(i, j int) bool {
		return strings.Count(markers[i], "/") > strings.Count(markers[j], "/")
	})

	ctx, cancel := deletionContext(ctx, fileStorageDeleteFolderTimeout)
	defer cancel()
	result := &domain.DeleteFolderResult{}
	for _, ok := range deleteKeys(ctx, store, bucket, files) {
		if ok {
			result.Deleted++
			result.DeletedFiles++
		} else {
			result.Failed++
			result.FailedFiles++
		}
	}
	for _, marker := range markers {
		if ctx.Err() == nil && deleteOne(ctx, store, bucket, marker) == nil {
			result.Deleted++
		} else {
			result.Failed++
		}
	}
	return result, nil
}

// TestSettings checks that the settings reach their bucket, for the owner.
func (s *FileStorageService) TestSettings(ctx context.Context, workspaceID string, settings domain.FileManagerSettings) error {
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if userWorkspace.Role != "owner" {
		return &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}
	if err := domain.ValidateStorageEndpoint(settings.Endpoint); err != nil {
		return err
	}
	if settings.Bucket == "" || settings.AccessKey == "" {
		return domain.NewValidationError("the bucket and the access key are required")
	}

	if settings.SecretKey == "" {
		workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
		if err != nil {
			return err
		}
		stored := workspace.Settings.FileManager
		if stored.SecretKey == "" || !domain.SameStorageAccount(settings, stored) {
			return domain.NewValidationError("enter the secret key: none is stored for this endpoint and access key")
		}
		settings.SecretKey = stored.SecretKey
	}

	store, err := s.stores(settings)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, fileStorageCallTimeout)
	defer cancel()
	if _, err := store.ListObjectsV2(callCtx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(settings.Bucket),
		MaxKeys: aws.Int32(1),
	}); err != nil {
		return storageError(err)
	}
	return nil
}
