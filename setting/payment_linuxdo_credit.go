package setting

const DefaultLinuxDoCreditBaseURL = "https://credit.linux.do/epay/pay"

var (
	LinuxDoCreditClientID     string
	LinuxDoCreditClientSecret string
	LinuxDoCreditBaseURL      string  = DefaultLinuxDoCreditBaseURL
	LinuxDoCreditUnitPrice    float64 = 1.0
	LinuxDoCreditMinTopUp     int     = 1
)
