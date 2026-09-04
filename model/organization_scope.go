package model

// OrganizationScoped identifies stored organization ownership. Global catalogs,
// channels, identities and unredeemed platform redemption codes are excluded
// from organization reads; platform queries must use a separate authorized API.
type OrganizationScoped interface {
	OrganizationID() int
}

func (v Token) OrganizationID() int             { return v.OrgId }
func (v Log) OrganizationID() int               { return v.OrgId }
func (v TopUp) OrganizationID() int             { return v.OrgId }
func (v UserSubscription) OrganizationID() int  { return v.OrgId }
func (v SubscriptionOrder) OrganizationID() int { return v.OrgId }
func (v Task) OrganizationID() int              { return v.OrgId }
func (v Midjourney) OrganizationID() int        { return v.OrgId }
func (v Redemption) OrganizationID() int {
	if v.OrgId == nil {
		return 0
	}
	return *v.OrgId
}
func (v QuotaData) OrganizationID() int                { return v.OrgId }
func (v OrganizationMember) OrganizationID() int       { return v.OrgId }
func (v OrganizationInvite) OrganizationID() int       { return v.OrgId }
func (v OrganizationTransfer) OrganizationID() int     { return v.OrgId }
func (v OrganizationAudit) OrganizationID() int        { return v.OrgId }
func (v OrganizationCharge) OrganizationID() int       { return v.OrgId }
func (v OrganizationNotification) OrganizationID() int { return v.OrgId }

// OrganizationResources is the complete persistence registry for scope regression
// checks. Log uses LOG_DB in production; the remaining models use DB.
func OrganizationResources() []OrganizationScoped {
	return []OrganizationScoped{&Token{}, &Log{}, &TopUp{}, &UserSubscription{}, &SubscriptionOrder{}, &Task{}, &Midjourney{}, &Redemption{}, &QuotaData{}, &OrganizationMember{}, &OrganizationInvite{}, &OrganizationTransfer{}, &OrganizationAudit{}, &OrganizationCharge{}, &OrganizationNotification{}}
}
