package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const linuxDoCreditUpstreamPayType = "epay"

func GetLinuxDoCreditClient() *epay.Client {
	if setting.LinuxDoCreditBaseURL == "" ||
		setting.LinuxDoCreditClientID == "" ||
		setting.LinuxDoCreditClientSecret == "" {
		return nil
	}
	client, err := epay.NewClient(&epay.Config{
		PartnerID: setting.LinuxDoCreditClientID,
		Key:       setting.LinuxDoCreditClientSecret,
	}, setting.LinuxDoCreditBaseURL)
	if err != nil {
		return nil
	}
	return client
}

func getLinuxDoCreditPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && ds > 0 {
		discount = ds
	}

	payMoney := dAmount.
		Mul(decimal.NewFromFloat(setting.LinuxDoCreditUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))

	return payMoney.InexactFloat64()
}

func formatLinuxDoCreditAmount(amount decimal.Decimal) string {
	return amount.StringFixed(2)
}

func normalizeLinuxDoCreditTopUpAmount(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}

	normalized := decimal.NewFromInt(amount).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		IntPart()
	if normalized < 1 {
		return 1
	}
	return normalized
}

func getLinuxDoCreditMinTopup() int64 {
	minTopup := int64(setting.LinuxDoCreditMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return decimal.NewFromInt(minTopup).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
			IntPart()
	}
	return minTopup
}

func newLinuxDoCreditPurchaseArgs(tradeNo string, amount int64, money string, notifyUrl *url.URL, returnUrl *url.URL) *epay.PurchaseArgs {
	return &epay.PurchaseArgs{
		Type:           linuxDoCreditUpstreamPayType,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", amount),
		Money:          money,
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	}
}

func RequestLinuxDoCreditAmount(c *gin.Context) {
	if !isLinuxDoCreditTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Linux DO Credit 支付未启用"})
		return
	}

	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	minTopup := getLinuxDoCreditMinTopup()
	if req.Amount < minTopup {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", minTopup)})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getLinuxDoCreditPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": formatLinuxDoCreditAmount(decimal.NewFromFloat(payMoney))})
}

func RequestLinuxDoCreditPay(c *gin.Context) {
	if !isLinuxDoCreditTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Linux DO Credit 支付未启用"})
		return
	}

	var req EpayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.PaymentMethod != "" && req.PaymentMethod != model.PaymentMethodLinuxDoCredit {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	minTopup := getLinuxDoCreditMinTopup()
	if req.Amount < minTopup {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", minTopup)})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getLinuxDoCreditPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	client := GetLinuxDoCreditClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置 Linux DO Credit 支付信息"})
		return
	}

	callbackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/console/log"))
	notifyUrl, _ := url.Parse(callbackAddress + "/api/user/linuxdo-credit/notify")
	tradeNo := fmt.Sprintf("LDC%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())
	money := formatLinuxDoCreditAmount(decimal.NewFromFloat(payMoney))

	uri, params, err := client.Purchase(newLinuxDoCreditPurchaseArgs(tradeNo, req.Amount, money, notifyUrl, returnUrl))
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Linux DO Credit 拉起支付失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          normalizeLinuxDoCreditTopUpAmount(req.Amount),
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodLinuxDoCredit,
		PaymentProvider: model.PaymentProviderLinuxDoCredit,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Linux DO Credit 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Linux DO Credit 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%s uri=%q params=%q", id, tradeNo, req.Amount, money, uri, common.GetJsonString(linuxDoCreditSafeLogParams(params))))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func LinuxDoCreditNotify(c *gin.Context) {
	if !isLinuxDoCreditWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	params := linuxDoCreditNotifyParams(c)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(linuxDoCreditSafeLogParams(params))))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	client := GetLinuxDoCreditClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Linux DO Credit client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.Type != linuxDoCreditUpstreamPayType {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 支付类型异常 trade_no=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)

	if err := model.RechargeLinuxDoCredit(verifyInfo.ServiceTradeNo, verifyInfo.Money, c.ClientIP()); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Linux DO Credit 充值回调处理失败 trade_no=%s callback_type=%s money=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.Money, c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Linux DO Credit 充值回调处理成功 trade_no=%s callback_type=%s money=%s client_ip=%s", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.Money, c.ClientIP()))
	_, _ = c.Writer.Write([]byte("success"))
}

func linuxDoCreditNotifyParams(c *gin.Context) map[string]string {
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Linux DO Credit webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			return nil
		}
		return lo.Reduce(lo.Keys(c.Request.PostForm), func(result map[string]string, key string, _ int) map[string]string {
			result[key] = c.Request.PostForm.Get(key)
			return result
		}, map[string]string{})
	}

	return lo.Reduce(lo.Keys(c.Request.URL.Query()), func(result map[string]string, key string, _ int) map[string]string {
		result[key] = c.Request.URL.Query().Get(key)
		return result
	}, map[string]string{})
}

func linuxDoCreditSafeLogParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return map[string]string{}
	}

	safe := map[string]string{}
	for _, key := range []string{"out_trade_no", "trade_no", "type", "trade_status", "money"} {
		if value := params[key]; value != "" {
			safe[key] = value
		}
	}
	return safe
}

func linuxDoCreditPayMethod() map[string]string {
	return map[string]string{
		"name":      "Linux DO Credit",
		"type":      model.PaymentMethodLinuxDoCredit,
		"color":     "black",
		"min_topup": strconv.Itoa(setting.LinuxDoCreditMinTopUp),
	}
}
