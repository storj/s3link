package s3link

import (
	"net/http"
	"net/http/httptest"
	"time"
	_ "unsafe"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"storj.io/gateway-mt/pkg/minio"
	"storj.io/gateway/miniogw"
	"storj.io/minio/pkg/auth"
	"storj.io/minio/pkg/env"
	"storj.io/uplink"
)

// NewHTTPClient returns a new http.Client with StorjTransport as the Transport.
// client := s3.New(awsSession, aws.NewConfig().WithHTTPClient(&s3link.NewHTTPClient()}))
// downloader := s3manager.NewDownloader(awsSession, func(d *s3manager.Downloader) { d.S3 = client })
// uploader := s3manager.NewUploader(awsSession, func(u *s3manager.Uploader) { u.S3 = client })
func NewHTTPClient() *http.Client {
	return &http.Client{Transport: NewStorjTransport()}
}

// StorjTransport can be used as an http.Transport, but uses the Storj Network directly via DRCP, not HTTP.
type StorjTransport struct {
	r *mux.Router
}

// NewStorjTransport can be used as an http.Client{Transport: NewStorjTransport()}.
// A serialized Storj access grant should be passed in via the environment variables "ACCESS".
func NewStorjTransport() StorjTransport {
	GlobalActiveCred = auth.Credentials{AccessKey: "placeholder", SecretKey: "placeholder"}
	GlobalServerRegion = "us-east-1"

	r := mux.NewRouter()
	r.SkipClean(true)
	r.UseEncodedPath()

	access, err := uplink.ParseAccess(env.Get("ACCESS", ""))
	if err != nil {
		panic("ACCESS env var not set")
	}
	s3Config := miniogw.S3CompatibilityConfig{
		IncludeCustomMetadataListing: true,
		MaxKeysLimit:                 1000,
		MaxKeysExhaustiveLimit:       100000,
		FullyCompatibleListing:       false,
		DisableCopyObject:            false,
		MinPartSize:                  5242880,
	}
	uplinkConfig := uplink.Config{DialTimeout: time.Minute * 2, UserAgent: "s3link/0.0.1"}
	gst := miniogw.NewSingleTenantGateway(zap.L(), access, uplinkConfig, miniogw.NewStorjGateway(s3Config), false)
	gatewayLayer, err := gst.NewGatewayLayer(auth.Credentials{})
	if err != nil {
		panic("error starting gateway layer")
	}
	minio.RegisterAPIRouter(r, gatewayLayer, []string{"s3.amazonaws.com"}, 5)
	return StorjTransport{r: r}
}

// RoundTrip implements the Transport interface.
func (n StorjTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	req.Header.Set("Host", "s3.amazonaws.com") // I'm not sure yet why this is needed
	rec := httptest.NewRecorder()
	n.r.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// GlobalActiveCred exposes minio's cmd.globalActiveCred.
//
//nolint: golint
//go:linkname GlobalActiveCred storj.io/minio/cmd.globalActiveCred
var GlobalActiveCred auth.Credentials

// GlobalServerRegion exposes minio's cmd.globalServerRegion.
//
//nolint: golint
//go:linkname GlobalServerRegion storj.io/minio/cmd.globalServerRegion
var GlobalServerRegion string
