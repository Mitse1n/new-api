package service

import "github.com/QuantumNous/new-api/model"

type OrganizationFunding struct {
	orgID     int
	userID    int
	tokenID   int
	requestID string
	receipt   *model.OrganizationCharge
}

func (funding *OrganizationFunding) Source() string {
	if funding.receipt != nil && funding.receipt.SubscriptionId > 0 {
		return BillingSourceSubscription
	}
	return BillingSourceWallet
}

func (funding *OrganizationFunding) PreConsume(amount int) error {
	receipt, err := model.ReserveOrganizationRequest(funding.orgID, funding.userID, funding.tokenID, funding.requestID, int64(amount))
	if err == nil {
		funding.receipt = receipt
	}
	return err
}

func (funding *OrganizationFunding) Settle(delta int) error {
	if funding.receipt == nil {
		return model.ErrOrganizationQuota
	}
	return model.FinalizeOrganizationCharge(funding.orgID, funding.requestID, funding.receipt.Quota+int64(delta), false)
}

func (funding *OrganizationFunding) Refund() error {
	if funding.receipt == nil {
		return nil
	}
	return refundWithRetry(func() error { return model.FinalizeOrganizationCharge(funding.orgID, funding.requestID, 0, true) })
}
