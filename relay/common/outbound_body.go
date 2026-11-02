package common

import (
	"io"
	"strings"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/common"
)

// NewOutboundJSONBody wraps the already-marshaled upstream request body into a
// BodyStorage. When disk cache is enabled and the payload exceeds the configured
// threshold, the data is written to a temp file and the original []byte can be
// GC'd, significantly reducing the heap residency while waiting for the
// upstream provider to respond (the dominant cost for large base64 payloads).
//
// In memory mode the underlying memoryStorage reuses the same backing array,
// so this is equivalent to bytes.NewReader(data) in terms of memory usage.
//
// The caller MUST invoke closer.Close() once the upstream call has finished
// (typically via defer) to release the disk file / memory accounting.
//
// The returned body exposes its size and replay capability without exposing
// io.Closer. Request construction uses that metadata to populate ContentLength
// and GetBody, while the caller retains ownership of the underlying storage
// through the separately returned closer.
func NewOutboundJSONBody(data []byte) (body common.ReplayableBody, closer io.Closer, err error) {
	storage, err := common.CreateBodyStorage(data)
	if err != nil {
		return nil, nil, err
	}
	return common.NewReplayableBodyReader(storage), storage, nil
}

// NewPassthroughJSONBody normalizes the two gateway-owned scalars without
// filtering raw request fields. An unchanged body borrows the incoming storage
// and returns a nil closer; a changed body owns separately replayable storage.
func NewPassthroughJSONBody(storage common.BodyStorage) (common.ReplayableBody, io.Closer, error) {
	data, err := storage.Bytes()
	if err != nil {
		return nil, nil, err
	}
	cacheKey := ojson.GetBytesNoCopy(data, "prompt_cache_key")
	tier := ojson.GetBytesNoCopy(data, "service_tier")
	normalizeCacheKey := cacheKey.Type == ojson.String && len(cacheKey.String()) > maxPromptCacheKeyLength
	normalizeTier := tier.Type == ojson.String && strings.EqualFold(strings.TrimSpace(tier.String()), "fast")
	if !normalizeCacheKey && !normalizeTier {
		return common.NewReplayableBodyReader(storage), nil, nil
	}
	data, err = normalizePromptCacheKeyAndFastServiceTier(ojson.NewDoc(data))
	if err != nil {
		return nil, nil, err
	}
	return NewOutboundJSONBody(data)
}
