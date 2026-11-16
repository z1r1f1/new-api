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
// The returned body reads through its own cursor and exposes its size and
// replay capability. Request construction uses that metadata to populate
// ContentLength and GetBody. Closing the body, as the transport does, releases
// only its cursor; the caller retains ownership of the underlying storage
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
	var data []byte
	var cacheKey, tier ojson.Result
	var err error
	if storage.IsDisk() {
		fields, readErr := common.ReadBodyJSONFields(storage, "prompt_cache_key", "service_tier")
		if readErr != nil {
			return nil, nil, readErr
		}
		cacheKey, tier = fields[0], fields[1]
	} else {
		data, err = storage.Bytes()
		if err != nil {
			return nil, nil, err
		}
		cacheKey = ojson.GetBytesNoCopy(data, "prompt_cache_key")
		tier = ojson.GetBytesNoCopy(data, "service_tier")
	}
	normalizeCacheKey := cacheKey.Type == ojson.String && len(cacheKey.String()) > maxPromptCacheKeyLength
	normalizeTier := tier.Type == ojson.String && strings.EqualFold(strings.TrimSpace(tier.String()), "fast")
	if !normalizeCacheKey && !normalizeTier {
		return common.NewReplayableBodyReader(storage), nil, nil
	}
	if data == nil {
		data, err = storage.Bytes()
		if err != nil {
			return nil, nil, err
		}
	}
	data, err = normalizePromptCacheKeyAndFastServiceTier(ojson.NewDoc(data))
	if err != nil {
		return nil, nil, err
	}
	return NewOutboundJSONBody(data)
}

// NormalizeGatewayJSONBody applies gateway-owned scalar invariants after all
// plugin, channel and adaptor edits, without filtering fields or rewriting an
// already-normalized body's bytes.
func NormalizeGatewayJSONBody(data []byte) ([]byte, error) {
	cacheKey := ojson.GetBytesNoCopy(data, "prompt_cache_key")
	tier := ojson.GetBytesNoCopy(data, "service_tier")
	if (cacheKey.Type != ojson.String || len(cacheKey.String()) <= maxPromptCacheKeyLength) &&
		(tier.Type != ojson.String || !strings.EqualFold(strings.TrimSpace(tier.String()), "fast")) {
		return data, nil
	}
	return normalizePromptCacheKeyAndFastServiceTier(ojson.NewDoc(data))
}
