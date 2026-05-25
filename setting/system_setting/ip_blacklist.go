package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type IPBlacklistSetting struct {
	Enabled bool   `json:"enabled"`
	List    string `json:"list"`
}

var ipBlacklistSetting = IPBlacklistSetting{
	Enabled: false,
	List:    "",
}

func init() {
	config.GlobalConfig.Register("ip_blacklist_setting", &ipBlacklistSetting)
}

func GetIPBlacklistSetting() *IPBlacklistSetting {
	return &ipBlacklistSetting
}
