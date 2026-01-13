package stats

import (
	"github.com/jpkribs/jellyfin-discovery-proxy/pkg/types"
)

// New creates a new RequestStats instance
func New() *types.RequestStats {
	return &types.RequestStats{}
}
