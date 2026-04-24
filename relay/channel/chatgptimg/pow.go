package chatgptimg

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/crypto/sha3"
)

const (
	powPrefixRequirements = "gAAAAAC"
	powPrefixProof        = "gAAAAAB"

	requirementsDifficulty = "0fffff"

	maxRequirementsIter = 500_000
	maxProofIter        = 100_000

	powFallback = "gAAAAABwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D"
)

var (
	powCores   = []int{16, 24, 32}
	powScreens = []int{3000, 4000, 6000}

	powNavKeys = []string{
		"webdriver−false", "vendor−Google Inc.", "cookieEnabled−true",
		"pdfViewerEnabled−true", "hardwareConcurrency−32",
		"language−zh-CN", "mimeTypes−[object MimeTypeArray]",
		"userAgentData−[object NavigatorUAData]",
	}
	powWinKeys = []string{
		"innerWidth", "innerHeight", "devicePixelRatio", "screen",
		"chrome", "location", "history", "navigator",
	}

	powReactListeners = []string{"_reactListeningcfilawjnerp", "_reactListening9ne2dfo1i47"}
	powProofEvents    = []string{"alert", "ontransitionend", "onprogress"}

	perfCounter uint64
)

type POWConfig struct {
	userAgent string
	arr       [18]any
}

func NewPOWConfig(userAgent string) *POWConfig {
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	//nolint:gosec // 非安全用途随机
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now().UTC()
	timeStr := now.Format("Mon Jan 02 2006 15:04:05") + " GMT+0000 (UTC)"
	perf := float64(atomic.AddUint64(&perfCounter, 1)) + rng.Float64()

	c := &POWConfig{userAgent: userAgent}
	c.arr = [18]any{
		powCores[rng.Intn(len(powCores))] + powScreens[rng.Intn(len(powScreens))],
		timeStr,
		nil,
		rng.Float64(),
		userAgent,
		nil,
		"dpl=1440a687921de39ff5ee56b92807faaadce73f13",
		"en-US",
		"en-US,zh-CN",
		0,
		powNavKeys[rng.Intn(len(powNavKeys))],
		"location",
		powWinKeys[rng.Intn(len(powWinKeys))],
		perf,
		randomUUID(rng),
		"",
		8,
		now.Unix(),
	}
	return c
}

func (c *POWConfig) RequirementsToken() string {
	seed := strconv.FormatFloat(rand.Float64(), 'f', -1, 64)
	b64, ok := c.solveRequirements(seed, requirementsDifficulty)
	if !ok {
		return powPrefixRequirements + powFallback + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	}
	return powPrefixRequirements + b64
}

func (c *POWConfig) solveRequirements(seed, difficulty string) (string, bool) {
	target, err := hex.DecodeString(difficulty)
	if err != nil {
		return "", false
	}
	diffLen := len(difficulty)

	arr := c.arr
	head, _ := common.Marshal([]any{arr[0], arr[1], arr[2]})
	p1 := append(head[:len(head)-1:len(head)-1], ',')

	mid, _ := common.Marshal([]any{arr[4], arr[5], arr[6], arr[7], arr[8]})
	p2 := make([]byte, 0, len(mid)+2)
	p2 = append(p2, ',')
	p2 = append(p2, mid[1:len(mid)-1]...)
	p2 = append(p2, ',')

	tail, _ := common.Marshal([]any{arr[10], arr[11], arr[12], arr[13], arr[14], arr[15], arr[16], arr[17]})
	p3 := make([]byte, 0, len(tail)+1)
	p3 = append(p3, ',')
	p3 = append(p3, tail[1:]...)

	hasher := sha3.New512()
	seedB := []byte(seed)
	buf := make([]byte, 0, len(p1)+32+len(p2)+16+len(p3))
	b64buf := make([]byte, base64.StdEncoding.EncodedLen(cap(buf)))

	for i := 0; i < maxRequirementsIter; i++ {
		d1 := strconv.Itoa(i)
		d2 := strconv.Itoa(i >> 1)

		buf = buf[:0]
		buf = append(buf, p1...)
		buf = append(buf, d1...)
		buf = append(buf, p2...)
		buf = append(buf, d2...)
		buf = append(buf, p3...)

		n := base64.StdEncoding.EncodedLen(len(buf))
		if cap(b64buf) < n {
			b64buf = make([]byte, n)
		}
		b64buf = b64buf[:n]
		base64.StdEncoding.Encode(b64buf, buf)

		hasher.Reset()
		hasher.Write(seedB)
		hasher.Write(b64buf)
		sum := hasher.Sum(nil)

		n2 := diffLen
		if n2 > len(sum) {
			n2 = len(sum)
		}
		cmpLen := n2
		if cmpLen > len(target) {
			cmpLen = len(target)
		}
		if bytes.Compare(sum[:cmpLen], target[:cmpLen]) <= 0 {
			return string(b64buf), true
		}
	}
	return "", false
}

func SolveProofToken(seed, difficulty, userAgent string) string {
	if seed == "" || difficulty == "" {
		return ""
	}
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	//nolint:gosec // 非安全用途随机
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	screen := powScreens[rng.Intn(len(powScreens))] * (1 << rng.Intn(3))
	timeStr := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")

	proofConfig := []any{
		screen,
		timeStr,
		nil,
		0,
		userAgent,
		"https://tcr9i.chat.openai.com/v2/35536E1E-65B4-4D96-9D97-6ADB7EFF8147/api.js",
		"dpl=1440a687921de39ff5ee56b92807faaadce73f13",
		"en",
		"en-US",
		nil,
		"plugins−[object PluginArray]",
		powReactListeners[rng.Intn(len(powReactListeners))],
		powProofEvents[rng.Intn(len(powProofEvents))],
	}

	diffLen := len(difficulty)
	hasher := sha3.New512()
	for i := 0; i < maxProofIter; i++ {
		proofConfig[3] = i
		raw, err := common.Marshal(proofConfig)
		if err != nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(raw)
		hasher.Reset()
		hasher.Write([]byte(seed + b64))
		sum := hasher.Sum(nil)
		hexStr := hex.EncodeToString(sum)
		if strings.Compare(hexStr[:diffLen], difficulty) <= 0 {
			return powPrefixProof + b64
		}
	}
	return powPrefixProof + powFallback + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
}

func randomUUID(rng *rand.Rand) string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rng.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToLower(hex.EncodeToString(b[0:4])) + "-" +
		strings.ToLower(hex.EncodeToString(b[4:6])) + "-" +
		strings.ToLower(hex.EncodeToString(b[6:8])) + "-" +
		strings.ToLower(hex.EncodeToString(b[8:10])) + "-" +
		strings.ToLower(hex.EncodeToString(b[10:16]))
}
