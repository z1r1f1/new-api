package controller

import (
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestGetLinuxDoCreditPayMoney(t *testing.T) {
	originalUnitPrice := setting.LinuxDoCreditUnitPrice
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()

	t.Cleanup(func() {
		setting.LinuxDoCreditUnitPrice = originalUnitPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	setting.LinuxDoCreditUnitPrice = 1.5
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{
		10:                           0.8,
		int(common.QuotaPerUnit * 3): 0.5,
		20:                           0,
	}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	testCases := []struct {
		name             string
		amount           int64
		group            string
		quotaDisplayType string
		expected         float64
	}{
		{
			name:             "currency display applies unit price group ratio and discount",
			amount:           10,
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         14.4,
		},
		{
			name:             "tokens display converts quota to display units before pricing",
			amount:           int64(common.QuotaPerUnit * 3),
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeTokens,
			expected:         2.7,
		},
		{
			name:             "non-positive discount falls back to no discount",
			amount:           20,
			group:            "default",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         30,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.quotaDisplayType
			actual := getLinuxDoCreditPayMoney(tc.amount, tc.group)
			require.InDelta(t, tc.expected, actual, 0.000001)
		})
	}
}

func TestFormatLinuxDoCreditAmountUsesTwoDecimalPlaces(t *testing.T) {
	require.Equal(t, "12.00", formatLinuxDoCreditAmount(decimal.NewFromInt(12)))
	require.Equal(t, "12.30", formatLinuxDoCreditAmount(decimal.RequireFromString("12.3")))
	require.Equal(t, "12.35", formatLinuxDoCreditAmount(decimal.RequireFromString("12.345")))
}

func TestLinuxDoCreditPurchaseUsesEasyPayCompatibilityParams(t *testing.T) {
	originalClientID := setting.LinuxDoCreditClientID
	originalClientSecret := setting.LinuxDoCreditClientSecret
	originalBaseURL := setting.LinuxDoCreditBaseURL
	t.Cleanup(func() {
		setting.LinuxDoCreditClientID = originalClientID
		setting.LinuxDoCreditClientSecret = originalClientSecret
		setting.LinuxDoCreditBaseURL = originalBaseURL
	})

	setting.LinuxDoCreditClientID = "ldc_client_id"
	setting.LinuxDoCreditClientSecret = "ldc_client_secret"
	setting.LinuxDoCreditBaseURL = "https://credit.linux.do/epay/pay"

	client := GetLinuxDoCreditClient()
	require.NotNil(t, client)

	notifyURL, err := url.Parse("https://example.com/api/user/linuxdo-credit/notify")
	require.NoError(t, err)
	returnURL, err := url.Parse("/console/log")
	require.NoError(t, err)

	purchaseArgs := newLinuxDoCreditPurchaseArgs("trade_no_123", 10, "12.30", notifyURL, returnURL)
	uri, params, err := client.Purchase(purchaseArgs)
	require.NoError(t, err)

	require.Equal(t, "https://credit.linux.do/epay/pay/submit.php", uri)
	require.Equal(t, "ldc_client_id", params["pid"])
	require.Equal(t, linuxDoCreditUpstreamPayType, params["type"])
	require.Equal(t, "trade_no_123", params["out_trade_no"])
	require.Equal(t, "TUC10", params["name"])
	require.Equal(t, "12.30", params["money"])
	require.Equal(t, "MD5", params["sign_type"])
	require.NotEmpty(t, params["sign"])
}
