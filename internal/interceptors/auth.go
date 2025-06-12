package interceptors

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var apiKeys = map[string]bool{"super-secret-api-key": true}

// ApiKeyInterceptor checks for API key validity
func ApiKeyInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	apiKeyHeaders := md.Get("x-api-key")
	if len(apiKeyHeaders) == 0 || !apiKeys[apiKeyHeaders[0]] {
		return status.Error(codes.Unauthenticated, "invalid API key")
	}

	return handler(srv, ss)
}
