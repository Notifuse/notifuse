module github.com/Notifuse/notifuse

go 1.25.4

require (
	contrib.go.opencensus.io/exporter/aws v0.0.0-20230502192102-15967c811cec
	contrib.go.opencensus.io/exporter/jaeger v0.2.1
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	contrib.go.opencensus.io/exporter/stackdriver v0.13.14
	contrib.go.opencensus.io/exporter/zipkin v0.1.2
	contrib.go.opencensus.io/integrations/ocsql v0.1.7
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/DataDog/opencensus-go-exporter-datadog v0.0.0-20220622145613-731d59e8b567
	github.com/Masterminds/squirrel v1.5.4
	github.com/Notifuse/liquidgo v0.0.0-20251124135804-bb1578ffeff3
	github.com/PuerkitoBio/goquery v1.10.3
	github.com/anthropics/anthropic-sdk-go v1.19.0
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2
	github.com/aws/aws-sdk-go v1.55.7
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.21.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.10.9
	github.com/lithammer/shortuuid/v4 v4.2.0
	github.com/openzipkin/zipkin-go v0.4.2
	github.com/preslavrachev/gomjml v0.10.0
	github.com/rs/zerolog v1.33.0
	github.com/spf13/viper v1.19.0
	github.com/standard-webhooks/standard-webhooks/libraries v0.0.0-20250711233419-a173a6c0125c
	github.com/stretchr/testify v1.9.0
	github.com/tidwall/gjson v1.18.0
	github.com/wneessen/go-mail v0.7.2
	go.opencensus.io v0.24.0
	golang.org/x/crypto v0.45.0
	golang.org/x/net v0.47.0
	golang.org/x/sync v0.18.0
	golang.org/x/time v0.6.0
)

require (
	cloud.google.com/go/auth v0.7.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.3 // indirect
	cloud.google.com/go/compute/metadata v0.5.0 // indirect
	cloud.google.com/go/monitoring v1.20.1 // indirect
	cloud.google.com/go/trace v1.10.9 // indirect
	github.com/DataDog/datadog-go v3.5.0+incompatible // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.5 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/prometheus v0.35.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tinylib/msgp v1.1.2 // indirect
	github.com/uber/jaeger-client-go v2.25.0+incompatible // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/otel v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.opentelemetry.io/otel/trace v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/api v0.189.0 // indirect
	google.golang.org/genproto v0.0.0-20240722135656-d784300faade // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240722135656-d784300faade // indirect
	google.golang.org/grpc v1.64.1 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.22.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Use Notifuse fork until PR is merged: https://github.com/preslavrachev/gomjml/pull/33
replace github.com/preslavrachev/gomjml => github.com/Notifuse/gomjml v0.0.0-20260130090101-a038317c31c2
