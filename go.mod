module github.com/savaki/aws-deployer

go 1.25.3

require (
	github.com/aws/aws-lambda-go v1.50.0
	github.com/aws/aws-sdk-go-v2 v1.39.3
	github.com/aws/aws-sdk-go-v2/config v1.31.13
	github.com/aws/aws-sdk-go-v2/credentials v1.18.17
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.67.1
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.51.1
	github.com/aws/aws-sdk-go-v2/service/iam v1.47.8
	github.com/aws/aws-sdk-go-v2/service/s3 v1.88.5
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.39.7
	github.com/aws/aws-sdk-go-v2/service/sfn v1.39.7
	github.com/aws/aws-sdk-go-v2/service/ssm v1.66.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.38.7
	github.com/aws/smithy-go v1.23.1
	github.com/awslabs/aws-lambda-go-api-proxy v0.16.2
	github.com/coreos/go-oidc/v3 v3.16.0
	github.com/gorilla/sessions v1.4.0
	github.com/graph-gophers/graphql-go v1.8.0
	github.com/open-policy-agent/opa v1.9.0
	github.com/rs/zerolog v1.34.0
	github.com/savaki/ddb/v2 v2.0.3
	github.com/savaki/gox v0.0.0-20251018200509-77d1208ddd07
	github.com/segmentio/ksuid v1.0.4
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v2 v2.27.6
	go.uber.org/dig v1.19.0
	golang.org/x/crypto v0.43.0
	golang.org/x/oauth2 v0.31.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.14.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.20.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.29.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.3.0 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/flatbuffers v25.9.23+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/dsig v1.0.0 // indirect
	github.com/lestrrat-go/dsig-secp256k1 v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc/v3 v3.0.1 // indirect
	github.com/lestrrat-go/jwx/v3 v3.0.11 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lestrrat-go/option/v2 v2.0.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.1 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/tchap/go-patricia/v2 v2.3.3 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/vektah/gqlparser/v2 v2.5.30 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.opentelemetry.io/proto/otlp v1.8.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/tools v0.37.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251014184007-4626949a642f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251014184007-4626949a642f // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
