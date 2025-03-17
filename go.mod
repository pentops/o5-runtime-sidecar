module github.com/pentops/o5-runtime-sidecar

go 1.24.0

toolchain go1.24.1

require (
	github.com/aws/aws-sdk-go-v2 v1.31.0
	github.com/aws/aws-sdk-go-v2/config v1.27.27
	github.com/aws/aws-sdk-go-v2/feature/rds/auth v1.4.18
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.33.3
	github.com/aws/aws-sdk-go-v2/service/sns v1.31.3
	github.com/aws/aws-sdk-go-v2/service/sqs v1.34.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.30.3
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/iancoleman/strcase v0.3.0
	github.com/jackc/pgx/v5 v5.5.1
	github.com/lib/pq v1.10.9
	github.com/pentops/envconf.go v0.0.0-20241008010024-9864aef6219d
	github.com/pentops/flowtest v0.0.0-20241110231021-42663ac00b63
	github.com/pentops/j5 v0.0.0-20250317192930-3df519f037f7
	github.com/pentops/jwtauth v0.0.0-20240806162904-172fc7d1f05b
	github.com/pentops/log.go v0.0.0-20250304233315-e0210b7a6dc3
	github.com/pentops/o5-messaging v0.0.0-20250317182016-de51c0e702a3
	github.com/pentops/runner v0.0.0-20250116202335-8635b2a42547
	github.com/pressly/goose v2.7.0+incompatible
	github.com/rs/cors v1.11.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.11.0
	google.golang.org/genproto/googleapis/api v0.0.0-20250106144421-5f5ef82da422
	google.golang.org/grpc v1.71.0
	google.golang.org/protobuf v1.36.5
	gopkg.in/square/go-jose.v2 v2.6.0
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.35.1-20240920164238-5a7b106cbb87.1 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.27 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.15 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.15 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.22.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.26.4 // indirect
	github.com/aws/smithy-go v1.21.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	golang.org/x/crypto v0.35.0 // indirect
	golang.org/x/exp v0.0.0-20241009180824-f66d83c29e7c // indirect
	golang.org/x/net v0.36.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
