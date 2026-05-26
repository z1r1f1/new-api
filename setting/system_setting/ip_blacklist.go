package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type IPBlacklistSetting struct {
	Enabled          bool   `json:"enabled"`
	List             string `json:"list"`
	AutoBanEnabled   bool   `json:"auto_ban_enabled"`
	AutoBanRpm       int    `json:"auto_ban_rpm"`
	AutoBanWhitelist string `json:"auto_ban_whitelist"`
}

var ipBlacklistSetting = IPBlacklistSetting{
	Enabled:          false,
	List:             "",
	AutoBanEnabled:   true,
	AutoBanRpm:       0,
	AutoBanWhitelist: "",
}

func init() {
	config.GlobalConfig.Register("ip_blacklist_setting", &ipBlacklistSetting)
}

func GetIPBlacklistSetting() *IPBlacklistSetting {
	return &ipBlacklistSetting
}
