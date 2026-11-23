package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	servicemocks "github.com/Notifuse/notifuse/internal/service/mocks"
	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

const fsWorkspaceID = "ws-files"

type fileStorageFixture struct {
	auth     *mocks.MockAuthService
	repo     *mocks.MockWorkspaceRepository
	store    *servicemocks.MockObjectStore
	svc      *FileStorageService
	settings []domain.FileManagerSettings // what the factory was given
}

func newFileStorageFixture(t *testing.T) *fileStorageFixture {
	ctrl := gomock.NewController(t)
	f := &fileStorageFixture{
		auth:  mocks.NewMockAuthService(ctrl),
		repo:  mocks.NewMockWorkspaceRepository(ctrl),
		store: servicemocks.NewMockObjectStore(ctrl),
	}
	f.svc = NewFileStorageService(f.auth, f.repo, func(settings domain.FileManagerSettings) (ObjectStore, error) {
		f.settings = append(f.settings, settings)
		return f.store, nil
	})
	f.svc.now = func() time.Time { return time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC) }
	return f
}

func configuredFileManager() domain.FileManagerSettings {
	return domain.FileManagerSettings{
		Endpoint:  "https://s3.example.com",
		Bucket:    "assets",
		AccessKey: "AKEXAMPLE",
		SecretKey: "stored-secret-key",
	}
}

// as authenticates the caller as uw, handing the caller's context on as the real service does.
func (f *fileStorageFixture) as(uw *domain.UserWorkspace) {
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), fsWorkspaceID).DoAndReturn(
		func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "u1"}, uw, nil
		})
}

func (f *fileStorageFixture) withSettings(settings domain.FileManagerSettings) {
	f.repo.EXPECT().GetByID(gomock.Any(), fsWorkspaceID).
		Return(&domain.Workspace{ID: fsWorkspaceID, Settings: domain.WorkspaceSettings{FileManager: settings}}, nil)
}

func owner() *domain.UserWorkspace { return &domain.UserWorkspace{Role: "owner"} }

func memberWith(perms domain.UserPermissions) *domain.UserWorkspace {
	return &domain.UserWorkspace{Role: "member", Permissions: perms}
}

func object(key string) types.Object {
	return types.Object{Key: aws.String(key), Size: aws.Int64(1), LastModified: aws.Time(time.Unix(0, 0))}
}

// Every operation checks the file rights before any repository or storage call: the strict
// mocks fail the test on an unexpected call.
func TestFileStorageService_Permissions(t *testing.T) {
	operations := map[string]struct {
		write bool
		call  func(svc *FileStorageService) error
	}{
		"List": {false, func(svc *FileStorageService) error {
			_, err := svc.List(context.Background(), fsWorkspaceID, "")
			return err
		}},
		"PresignUpload": {true, func(svc *FileStorageService) error {
			_, err := svc.PresignUpload(context.Background(), domain.PresignUploadRequest{WorkspaceID: fsWorkspaceID, Key: "a.png", ContentType: "image/png"})
			return err
		}},
		"CreateFolder": {true, func(svc *FileStorageService) error {
			_, err := svc.CreateFolder(context.Background(), fsWorkspaceID, "folder/")
			return err
		}},
		"Delete": {true, func(svc *FileStorageService) error {
			_, err := svc.Delete(context.Background(), fsWorkspaceID, []string{"a.png"})
			return err
		}},
		"DeleteFolder": {true, func(svc *FileStorageService) error {
			_, err := svc.DeleteFolder(context.Background(), fsWorkspaceID, "folder/")
			return err
		}},
	}
	refusedFor := map[string]struct {
		uw          *domain.UserWorkspace
		refuseRead  bool
		refuseWrite bool
	}{
		"no rights":      {memberWith(domain.UserPermissions{}), true, true},
		"contacts only":  {memberWith(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}}), true, true},
		"templates read": {memberWith(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true}}), false, true},
	}

	for opName, op := range operations {
		for who, rights := range refusedFor {
			refused := rights.refuseRead
			if op.write {
				refused = rights.refuseWrite
			}
			if !refused {
				continue
			}
			t.Run(opName+"/"+who, func(t *testing.T) {
				f := newFileStorageFixture(t)
				f.as(rights.uw)

				err := op.call(f.svc)
				var unauthorized *domain.ErrUnauthorized
				require.ErrorAs(t, err, &unauthorized)
				assert.Contains(t, unauthorized.Message, "templates or the blog")
				assert.Empty(t, f.settings, "no storage client may be built for a refused caller")
			})
		}
	}
}

func TestFileStorageService_BlogWriterMayUploadAndTemplatesReaderMayList(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(memberWith(domain.UserPermissions{domain.PermissionResourceBlog: {Write: true}}))
	f.withSettings(configuredFileManager())
	f.store.EXPECT().PresignPutObject(gomock.Any(), gomock.Any(), domain.StorageUploadURLExpiry).
		Return(&v4.PresignedHTTPRequest{URL: "https://signed"}, nil)
	_, err := f.svc.PresignUpload(context.Background(), domain.PresignUploadRequest{WorkspaceID: fsWorkspaceID, Key: "a.png"})
	require.NoError(t, err)

	g := newFileStorageFixture(t)
	g.as(memberWith(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true}}))
	g.withSettings(configuredFileManager())
	g.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(&s3.ListObjectsV2Output{}, nil)
	_, err = g.svc.List(context.Background(), fsWorkspaceID, "")
	require.NoError(t, err)
}

func TestFileStorageService_NotConfigured(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(domain.FileManagerSettings{})

	_, err := f.svc.List(context.Background(), fsWorkspaceID, "")
	var validation domain.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Empty(t, f.settings)
}

func TestFileStorageService_ListFollowsPages(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	gomock.InOrder(
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				assert.Nil(t, in.ContinuationToken)
				assert.Equal(t, "images/", aws.ToString(in.Prefix))
				assert.Equal(t, "assets", aws.ToString(in.Bucket))
				return &s3.ListObjectsV2Output{Contents: []types.Object{object("images/a.png")}, IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("t1")}, nil
			}),
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				assert.Equal(t, "t1", aws.ToString(in.ContinuationToken))
				return &s3.ListObjectsV2Output{Contents: []types.Object{object("images/b.png")}, IsTruncated: aws.Bool(false)}, nil
			}),
	)

	result, err := f.svc.List(context.Background(), fsWorkspaceID, "images/")
	require.NoError(t, err)
	require.Len(t, result.Objects, 2)
	assert.Equal(t, "images/b.png", result.Objects[1].Key)
	assert.False(t, result.Capped)
	assert.False(t, result.CutShort)
}

func TestFileStorageService_ListCutShortWithoutToken(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).
		Return(&s3.ListObjectsV2Output{Contents: []types.Object{object("a")}, IsTruncated: aws.Bool(true)}, nil)

	result, err := f.svc.List(context.Background(), fsWorkspaceID, "")
	require.NoError(t, err)
	assert.True(t, result.CutShort, "a provider that says there is more without a token")
	assert.Len(t, result.Objects, 1)
}

func TestFileStorageService_ListCappedAtTheMaximum(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	page := make([]types.Object, 1000)
	for i := range page {
		page[i] = object(fmt.Sprintf("k%05d", i))
	}
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).
		Return(&s3.ListObjectsV2Output{Contents: page, IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("next")}, nil).
		Times(domain.MaxListedStorageObjects / 1000)

	result, err := f.svc.List(context.Background(), fsWorkspaceID, "")
	require.NoError(t, err)
	assert.True(t, result.Capped)
	assert.Len(t, result.Objects, domain.MaxListedStorageObjects)
}

func TestFileStorageService_ListStopsAtThePageLimit(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).
		Return(&s3.ListObjectsV2Output{IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("next")}, nil).
		Times(fileStorageMaxListPages)

	result, err := f.svc.List(context.Background(), fsWorkspaceID, "")
	require.NoError(t, err)
	assert.True(t, result.CutShort, "a provider that pages forever with empty pages")
}

func TestFileStorageService_ListReportsTheProviderError(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(nil, errors.New("dial tcp: no such host"))

	_, err := f.svc.List(context.Background(), fsWorkspaceID, "")
	var storageErr *domain.FileStorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Contains(t, storageErr.Message, "no such host")
}

// The real SDK client, without network: the URL is signed locally.
func TestFileStorageService_PresignUploadSignsOnlyTheTypeAndTheHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := mocks.NewMockAuthService(ctrl)
	repo := mocks.NewMockWorkspaceRepository(ctrl)
	svc := NewFileStorageService(auth, repo, NewS3ObjectStoreFactory(nil))
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), fsWorkspaceID).Return(context.Background(), &domain.User{}, owner(), nil)
	repo.EXPECT().GetByID(gomock.Any(), fsWorkspaceID).
		Return(&domain.Workspace{Settings: domain.WorkspaceSettings{FileManager: configuredFileManager()}}, nil)

	upload, err := svc.PresignUpload(context.Background(), domain.PresignUploadRequest{
		WorkspaceID: fsWorkspaceID, Key: "images/logo@2x.png", ContentType: "image/png", Size: 1234,
	})
	require.NoError(t, err)

	u, err := url.Parse(upload.URL)
	require.NoError(t, err)
	query := u.Query()
	assert.Equal(t, "content-type;host", query.Get("X-Amz-SignedHeaders"), "no content-length, no checksum header")
	assert.Equal(t, "300", query.Get("X-Amz-Expires"))
	for name := range query {
		assert.False(t, strings.HasPrefix(strings.ToLower(name), "x-amz-checksum"), "unexpected %s", name)
		assert.NotEqual(t, "x-amz-sdk-checksum-algorithm", strings.ToLower(name))
	}
	assert.Equal(t, "assets.s3.example.com", u.Host, "without path style, the bucket is in the host")
	assert.Equal(t, "/images/logo%402x.png", u.EscapedPath())
	assert.Equal(t, map[string]string{"Content-Type": "image/png"}, upload.Headers)
	assert.NotContains(t, upload.URL, "stored-secret-key")
}

func TestFileStorageService_PresignUploadDefaultsTheContentType(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().PresignPutObject(gomock.Any(), gomock.Any(), domain.StorageUploadURLExpiry).DoAndReturn(
		func(_ context.Context, in *s3.PutObjectInput, _ time.Duration) (*v4.PresignedHTTPRequest, error) {
			assert.Equal(t, "application/octet-stream", aws.ToString(in.ContentType))
			assert.Nil(t, in.ContentLength, "the length is never signed")
			return &v4.PresignedHTTPRequest{URL: "https://signed"}, nil
		})

	upload, err := f.svc.PresignUpload(context.Background(), domain.PresignUploadRequest{WorkspaceID: fsWorkspaceID, Key: "font.woff2"})
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", upload.Headers["Content-Type"])
	assert.Equal(t, time.Date(2026, 11, 23, 10, 5, 0, 0, time.UTC), upload.ExpiresAt)
}

// Invalid input is refused before anyone is authenticated or any client is built.
func TestFileStorageService_PresignUploadRefusesBadInput(t *testing.T) {
	for name, req := range map[string]domain.PresignUploadRequest{
		"folder key":   {Key: "images/"},
		"dot segment":  {Key: "a/../b.png"},
		"empty key":    {Key: ""},
		"bad type":     {Key: "a.png", ContentType: "image/png\r\nX: 1"},
		"too long key": {Key: strings.Repeat("a", 1025)},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFileStorageFixture(t)
			req.WorkspaceID = fsWorkspaceID
			_, err := f.svc.PresignUpload(context.Background(), req)
			var validation domain.ValidationError
			require.ErrorAs(t, err, &validation)
		})
	}
}

func TestFileStorageService_CreateFolder(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.withSettings(configuredFileManager())
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
				assert.Equal(t, int32(1), aws.ToInt32(in.MaxKeys))
				assert.Equal(t, "photos/", aws.ToString(in.Prefix))
				return &s3.ListObjectsV2Output{Contents: []types.Object{object("photos/a.png")}}, nil
			})

		result, err := f.svc.CreateFolder(context.Background(), fsWorkspaceID, "photos/")
		require.NoError(t, err)
		assert.Equal(t, domain.FolderExists, result.Status)
	})
	t.Run("created", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.withSettings(configuredFileManager())
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(&s3.ListObjectsV2Output{}, nil)
		f.store.EXPECT().PutObject(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
				assert.Equal(t, "photos/", aws.ToString(in.Key))
				return &s3.PutObjectOutput{}, nil
			})

		result, err := f.svc.CreateFolder(context.Background(), fsWorkspaceID, "photos/")
		require.NoError(t, err)
		assert.Equal(t, domain.FolderCreated, result.Status)
	})
	t.Run("not a folder", func(t *testing.T) {
		f := newFileStorageFixture(t)
		_, err := f.svc.CreateFolder(context.Background(), fsWorkspaceID, "photos")
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
	})
}

func TestFileStorageService_DeleteCountsFailuresAndDeduplicates(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	var mu sync.Mutex
	var deleted []string
	f.store.EXPECT().DeleteObject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			mu.Lock()
			deleted = append(deleted, aws.ToString(in.Key))
			mu.Unlock()
			if aws.ToString(in.Key) == "locked.png" {
				return nil, errors.New("AccessDenied")
			}
			return &s3.DeleteObjectOutput{}, nil
		}).Times(3)

	result, err := f.svc.Delete(context.Background(), fsWorkspaceID, []string{"a.png", "locked.png", "/legacy//b.png", "a.png"})
	require.NoError(t, err)
	assert.Equal(t, &domain.DeleteFilesResult{Deleted: 2, Failed: 1}, result)
	assert.ElementsMatch(t, []string{"a.png", "locked.png", "/legacy//b.png"}, deleted, "keys other tools created stay deletable")
}

// A client that goes away, or a proxy that times the request out, does not stop the deletes.
func TestFileStorageService_DeletesOutliveTheRequest(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).
		Return(&s3.ListObjectsV2Output{Contents: []types.Object{object("a/x.png"), object("a/")}}, nil)
	f.store.EXPECT().DeleteObject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			assert.NoError(t, ctx.Err(), "the delete runs on its own budget")
			return &s3.DeleteObjectOutput{}, nil
		}).Times(2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := f.svc.DeleteFolder(ctx, fsWorkspaceID, "a/")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Deleted)
}

func TestFileStorageService_DeleteRunsConcurrently(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	var mu sync.Mutex
	inFlight, peak := 0, 0
	release := make(chan struct{})
	var opened sync.Once
	f.store.EXPECT().DeleteObject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			mu.Lock()
			inFlight++
			if inFlight > peak {
				peak = inFlight
			}
			reached := peak == fileStorageDeleteConcurrency
			mu.Unlock()
			if reached {
				opened.Do(func() { close(release) })
			}
			<-release
			mu.Lock()
			inFlight--
			mu.Unlock()
			return &s3.DeleteObjectOutput{}, nil
		}).Times(20)

	keys := make([]string, 20)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
	}
	result, err := f.svc.Delete(context.Background(), fsWorkspaceID, keys)
	require.NoError(t, err)
	assert.Equal(t, 20, result.Deleted)
	assert.Equal(t, fileStorageDeleteConcurrency, peak, "deletes run in parallel, bounded")
}

func TestFileStorageService_DeleteRefusesTooManyKeys(t *testing.T) {
	f := newFileStorageFixture(t)
	keys := make([]string, domain.MaxDeletedStorageKeys+1)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
	}
	_, err := f.svc.Delete(context.Background(), fsWorkspaceID, keys)
	var validation domain.ValidationError
	require.ErrorAs(t, err, &validation)

	_, err = f.svc.Delete(context.Background(), fsWorkspaceID, nil)
	require.ErrorAs(t, err, &validation)
}

func TestFileStorageService_DeleteFolderDeletesFilesBeforeTheirFolders(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	// Pre-order, as a filesystem walk lists: folders before their content.
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(&s3.ListObjectsV2Output{Contents: []types.Object{
		object("a/"), object("a/b/"), object("a/b/deep.png"), object("a/top.png"), object("ab-sibling.png"),
	}}, nil)
	var mu sync.Mutex
	var order []string
	f.store.EXPECT().DeleteObject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
			mu.Lock()
			order = append(order, aws.ToString(in.Key))
			mu.Unlock()
			if aws.ToString(in.Key) == "a/top.png" {
				return nil, errors.New("AccessDenied")
			}
			return &s3.DeleteObjectOutput{}, nil
		}).Times(4)

	result, err := f.svc.DeleteFolder(context.Background(), fsWorkspaceID, "a/")
	require.NoError(t, err)
	require.Len(t, order, 4)
	assert.ElementsMatch(t, []string{"a/b/deep.png", "a/top.png"}, order[:2], "files first, in parallel")
	assert.Equal(t, []string{"a/b/", "a/"}, order[2:],
		"then markers from the deepest; an object outside the prefix is left alone")
	assert.Equal(t, &domain.DeleteFolderResult{Deleted: 3, Failed: 1, DeletedFiles: 1, FailedFiles: 1}, result)
}

func TestFileStorageService_DeleteFolderRefusesAPartialListing(t *testing.T) {
	f := newFileStorageFixture(t)
	f.as(owner())
	f.withSettings(configuredFileManager())
	f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).
		Return(&s3.ListObjectsV2Output{Contents: []types.Object{object("a/x")}, IsTruncated: aws.Bool(true)}, nil)

	_, err := f.svc.DeleteFolder(context.Background(), fsWorkspaceID, "a/")
	var storageErr *domain.FileStorageError
	require.ErrorAs(t, err, &storageErr, "no DeleteObject call is expected")
}

func TestFileStorageService_DeleteFolderRefusesTheWholeBucket(t *testing.T) {
	f := newFileStorageFixture(t)
	for _, prefix := range []string{"", "a", "/a/"} {
		_, err := f.svc.DeleteFolder(context.Background(), fsWorkspaceID, prefix)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation, prefix)
	}
}

func TestFileStorageService_TestSettings(t *testing.T) {
	t.Run("member refused", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(memberWith(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}}))
		err := f.svc.TestSettings(context.Background(), fsWorkspaceID, configuredFileManager())
		var unauthorized *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauthorized)
		assert.Empty(t, f.settings)
	})
	t.Run("empty secret uses the stored one", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.withSettings(configuredFileManager())
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(&s3.ListObjectsV2Output{}, nil)

		form := configuredFileManager()
		form.SecretKey = ""
		form.Bucket = "another-bucket"
		require.NoError(t, f.svc.TestSettings(context.Background(), fsWorkspaceID, form))
		require.Len(t, f.settings, 1)
		assert.Equal(t, "stored-secret-key", f.settings[0].SecretKey)
		assert.Equal(t, "another-bucket", f.settings[0].Bucket)
	})
	t.Run("given secret is used", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(&s3.ListObjectsV2Output{}, nil)

		form := configuredFileManager()
		form.SecretKey = "typed-secret"
		require.NoError(t, f.svc.TestSettings(context.Background(), fsWorkspaceID, form))
		assert.Equal(t, "typed-secret", f.settings[0].SecretKey)
	})
	t.Run("stored secret not sent to another host", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.withSettings(configuredFileManager())

		form := configuredFileManager()
		form.SecretKey = ""
		form.Endpoint = "https://collector.example.net"
		err := f.svc.TestSettings(context.Background(), fsWorkspaceID, form)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Empty(t, f.settings, "nothing may be signed with the stored secret for another host")
	})
	t.Run("not http", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		form := configuredFileManager()
		form.Endpoint = "file:///etc/passwd"
		err := f.svc.TestSettings(context.Background(), fsWorkspaceID, form)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
	})
	t.Run("provider error", func(t *testing.T) {
		f := newFileStorageFixture(t)
		f.as(owner())
		f.store.EXPECT().ListObjectsV2(gomock.Any(), gomock.Any()).Return(nil, errors.New("SignatureDoesNotMatch"))
		form := configuredFileManager()
		err := f.svc.TestSettings(context.Background(), fsWorkspaceID, form)
		var storageErr *domain.FileStorageError
		require.ErrorAs(t, err, &storageErr)
	})
}

// With the SSRF-protected client the app passes by default, a private endpoint is refused
// at dial time, and the error names the setting that allows it.
func TestFileStorageService_PrivateEndpointIsRefusedByDefault(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:9", "http://169.254.169.254", "http://10.0.0.5:6379"} {
		t.Run(endpoint, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			auth := mocks.NewMockAuthService(ctrl)
			repo := mocks.NewMockWorkspaceRepository(ctrl)
			svc := NewFileStorageService(auth, repo, NewS3ObjectStoreFactory(safehttpclient.New()))
			auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), fsWorkspaceID).Return(context.Background(), &domain.User{}, owner(), nil)

			settings := configuredFileManager()
			settings.Endpoint = endpoint
			err := svc.TestSettings(context.Background(), fsWorkspaceID, settings)
			var storageErr *domain.FileStorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Contains(t, storageErr.Message, "FILE_STORAGE_ALLOW_PRIVATE_HOSTS")
		})
	}
}

type capturingTransport struct{ req *http.Request }

func (c *capturingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.req = r
	return nil, errors.New("captured")
}

// Google Cloud Storage refuses a signature that covers Accept-Encoding: the header is not signed.
func TestS3ObjectStore_DoesNotSignAcceptEncoding(t *testing.T) {
	transport := &capturingTransport{}
	settings := configuredFileManager()
	settings.Endpoint = "https://storage.googleapis.com"
	store, err := NewS3ObjectStoreFactory(&http.Client{Transport: transport})(settings)
	require.NoError(t, err)

	_, _ = store.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String("assets")})
	require.NotNil(t, transport.req)
	authorization := transport.req.Header.Get("Authorization")
	require.Contains(t, authorization, "SignedHeaders=")
	assert.NotContains(t, strings.ToLower(authorization), "accept-encoding")
}
