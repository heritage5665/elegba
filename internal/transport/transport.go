// File path: elegba/internal/transport/transport.go

package transport

import (
	"context"
	"net/http"
)

type Transport interface {
	Do(context.Context, *http.Request) (*http.Response, error)
}
