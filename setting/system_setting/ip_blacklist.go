package system_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	IPAutoBanScopeAll    = "all"
	IPAutoBanScopeRelay  = "relay"
	IPAutoBanScopeCustom = "custom"
)

type IPBlacklistSetting struct {
	Enabled             bool   `json:"enabled"`
	List                string `json:"list"`
	AutoBanEnabled      bool   `json:"auto_ban_enabled"`
	AutoBanRpm          int    `json:"auto_ban_rpm"`
	AutoBanWhitelist    string `json:"auto_ban_whitelist"`
	AutoBanScope        string `json:"auto_ban_scope"`
	AutoBanPathPrefixes string `json:"auto_ban_path_prefixes"`
}

var ipBlacklistSetting = IPBlacklistSetting{
	Enabled:             false,
	List:                "",
	AutoBanEnabled:      true,
	AutoBanRpm:          0,
	AutoBanWhitelist:    "",
	AutoBanScope:        IPAutoBanScopeRelay,
	AutoBanPathPrefixes: "",
}

func init() {
	config.GlobalConfig.Register("ip_blacklist_setting", &ipBlacklistSetting)
}

func GetIPBlacklistSetting() *IPBlacklistSetting {
	return &ipBlacklistSetting
}
