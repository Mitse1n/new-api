package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/waffo-com/waffo-go/types/order"
)

func SubscriptionRequestWaffoPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !setting.WaffoEnabled {
		common.ApiErrorMsg(c, "Waffo 支付未启用")
		return
	}
	var req struct {
		PlanId         int  `json:"plan_id"`
		PayMethodIndex *int `json:"pay_method_index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled || plan.PriceAmount <= 0 {
		common.ApiErrorMsg(c, "套餐不可购买")
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payment := &order.PaymentInfo{ProductName: "ONE_TIME_PAYMENT"}
	if req.PayMethodIndex != nil {
		methods := setting.GetWaffoPayMethods()
		index := *req.PayMethodIndex
		if index < 0 || index >= len(methods) {
			common.ApiErrorMsg(c, "不支持的支付方式")
			return
		}
		payment.PayMethodType, payment.PayMethodName = methods[index].PayMethodType, methods[index].PayMethodName
	}
	sdk, err := getWaffoSDK()
	if err != nil {
		common.ApiErrorMsg(c, "支付配置错误")
		return
	}
	tradeNo := fmt.Sprintf("WAFFO_SUB-%d-%s", time.Now().UnixMilli(), common.GetRandomString(12))
	purchase := &model.SubscriptionOrder{OrgId: c.GetInt("org_id"), UserId: user.Id, PlanId: plan.Id,
		Money: plan.PriceAmount, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodWaffo,
		PaymentProvider: model.PaymentProviderWaffo, Status: common.TopUpStatusPending}
	if err := purchase.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	notifyURL := service.GetCallbackAddress() + "/api/waffo/webhook"
	if setting.WaffoNotifyUrl != "" {
		notifyURL = setting.WaffoNotifyUrl
	}
	returnURL := paymentReturnPath("/organization/plans")
	params := &order.CreateOrderParams{PaymentRequestID: tradeNo, MerchantOrderID: tradeNo,
		OrderAmount: decimal.NewFromFloat(purchase.Money).StringFixed(2), OrderCurrency: "USD",
		OrderDescription: plan.Title, OrderRequestedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), NotifyURL: notifyURL,
		MerchantInfo: &order.MerchantInfo{MerchantID: setting.WaffoMerchantId},
		UserInfo:     &order.UserInfo{UserID: strconv.Itoa(user.Id), UserEmail: getWaffoUserEmail(user), UserTerminal: "WEB"},
		PaymentInfo:  payment, GoodsInfo: &order.GoodsInfo{GoodsName: plan.Title, AppName: "New API"},
		SuccessRedirectURL: returnURL, FailedRedirectURL: returnURL}
	response, err := sdk.Order().Create(c.Request.Context(), params, nil)
	if err != nil || !response.IsSuccess() {
		purchase.Status = common.TopUpStatusFailed
		_ = purchase.Update()
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	data := response.GetData()
	paymentURL := data.FetchRedirectURL()
	if paymentURL == "" {
		paymentURL = data.OrderAction
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"checkout_url": paymentURL, "order_id": tradeNo}})
}
