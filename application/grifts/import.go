package grifts

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/gobuffalo/pop/v5"
	"github.com/gofrs/uuid"
	"github.com/markbates/grift/grift"

	"github.com/silinternational/riskman-api/api"
	"github.com/silinternational/riskman-api/domain"
	"github.com/silinternational/riskman-api/models"
)

/*
	This is a temporary command-line utility to read a JSON file with data from the legacy Riskman software.

	The input file is expected to have a number of top-level objects, as defined in `LegacyData`. The `Policies`
	list is a complex structure contained related data. The remainder are simple objects.

TODO:
	1. Import users and assign correct IDs (claim.reviewer_id)
	2. Import policy members (parse email field on polices)
	3. Import other tables (e.g. Journal Entries)
*/

const (
	TimeFormat = "2006-01-02 15:04:05"
	EmptyTime  = "1970-01-01 00:00:00"
)

type LegacyData struct {
	Users          []LegacyUsers        `json:"users"`
	Policies       []LegacyPolicy       `json:"policies"`
	PolicyTypes    []PolicyType         `json:"PolicyType"`
	Maintenance    []Maintenance        `json:"Maintenance"`
	JournalEntries []JournalEntry       `json:"tblJEntry"`
	ItemCategories []LegacyItemCategory `json:"item_categories"`
	RiskCategories []LegacyRiskCategory `json:"risk_categories"`
	LossReasons    []LossReason         `json:"LossReason"`
}

type LegacyUsers struct {
	Location      string `json:"location"`
	CreatedAt     string `json:"created_at"`
	LastName      string `json:"last_name"`
	FirstName     string `json:"first_name"`
	UpdatedAt     string `json:"updated_at"`
	Id            string `json:"id"`
	Email         string `json:"email"`
	EmailOverride string `json:"email_override"`
	IsBlocked     int    `json:"is_blocked"`
	LastLoginUtc  string `json:"last_login_utc"`
	StaffId       string `json:"staff_id"`
}

type LegacyPolicy struct {
	Id          string        `json:"id"`
	Claims      []LegacyClaim `json:"claims"`
	Notes       string        `json:"notes"`
	IdentCode   string        `json:"ident_code"`
	CostCenter  string        `json:"cost_center"`
	Items       []LegacyItem  `json:"items"`
	Account     int           `json:"account"`
	HouseholdId string        `json:"household_id"`
	EntityCode  nulls.String  `json:"entity_code"`
	Type        string        `json:"type"`
	UpdatedAt   nulls.String  `json:"updated_at"`
	CreatedAt   string        `json:"created_at"`
}

type LegacyItem struct {
	PolicyId          int          `json:"policy_id"`
	InStorage         int          `json:"in_storage"`
	PurchaseDate      string       `json:"purchase_date"`
	Name              string       `json:"name"`
	CoverageStartDate string       `json:"coverage_start_date"`
	Make              string       `json:"make"`
	Description       string       `json:"description"`
	SerialNumber      string       `json:"serial_number"`
	CreatedAt         string       `json:"created_at"`
	Id                string       `json:"id"`
	Country           string       `json:"country"`
	Model             string       `json:"model"`
	CategoryId        int          `json:"category_id"`
	CoverageAmount    string       `json:"coverage_amount"`
	UpdatedAt         nulls.String `json:"updated_at"`
	PolicyDependentId int          `json:"policy_dependent_id"`
	CoverageStatus    string       `json:"coverage_status"`
}

type LegacyItemCategory struct {
	CreatedAt      string `json:"created_at"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
	AutoApproveMax string `json:"auto_approve_max"`
	RiskCategoryId int    `json:"risk_category_id"`
	HelpText       string `json:"help_text"`
	Id             string `json:"id"`
}

type LegacyClaim struct {
	PolicyId         int               `json:"policy_id"`
	ReviewerId       int               `json:"reviewer_id"`
	ClaimItems       []LegacyClaimItem `json:"claim_items"`
	CreatedAt        string            `json:"created_at"`
	ReviewDate       string            `json:"review_date"`
	UpdatedAt        string            `json:"updated_at"`
	EventDescription string            `json:"event_description"`
	Id               string            `json:"id"`
	EventType        string            `json:"event_type"`
	PaymentDate      string            `json:"payment_date"`
	TotalPayout      string            `json:"total_payout"`
	EventDate        string            `json:"event_date"`
	Status           string            `json:"status"`
}

type LegacyClaimItem struct {
	ReviewerId      int    `json:"reviewer_id"`
	PayoutAmount    string `json:"payout_amount"`
	ItemId          int    `json:"item_id"`
	PayoutOption    string `json:"payout_option"`
	RepairEstimate  string `json:"repair_estimate"`
	UpdatedAt       string `json:"updated_at"`
	ReviewDate      string `json:"review_date"`
	CreatedAt       string `json:"created_at"`
	Location        string `json:"location"`
	ReplaceActual   string `json:"replace_actual"`
	Id              string `json:"id"`
	ReplaceEstimate string `json:"replace_estimate"`
	IsRepairable    int    `json:"is_repairable"`
	Status          string `json:"status"`
	RepairActual    string `json:"repair_actual"`
	Fmv             string `json:"fmv"`
	ClaimId         int    `json:"claim_id"`
}

type PolicyType struct {
	MusicMax        int     `json:"Music_Max"`
	MinRefund       int     `json:"Min_Refund"`
	PolTypeRecNum   string  `json:"PolType_Rec_Num"`
	WarLimit        float64 `json:"War_Limit"`
	PolicyDeductMin int     `json:"Policy_Deduct_Min"`
	PolicyRate      float64 `json:"Policy_Rate"`
	PolicyType      string  `json:"Policy_Type"`
	MinFee          int     `json:"Min_Fee"`
	PolicyDeductPct float64 `json:"Policy_Deduct_Pct"`
}

type LegacyRiskCategory struct {
	CreatedAt string       `json:"created_at"`
	Name      string       `json:"name"`
	PolicyMax nulls.Int    `json:"policy_max"`
	UpdatedAt nulls.String `json:"updated_at"`
	Id        string       `json:"id"`
}

type LossReason struct {
	ReasonRecNum string `json:"Reason_Rec_Num"`
	Reason       string `json:"Reason"`
}

type Maintenance struct {
	DateResolved nulls.String `json:"dateResolved"`
	Seq          string       `json:"seq"`
	Problem      string       `json:"problem"`
	DateReported string       `json:"dateReported"`
	Resolution   nulls.String `json:"resolution"`
}

type JournalEntry struct {
	FirstName   nulls.String `json:"First_Name"`
	JERecNum    string       `json:"JE_Rec_Num"`
	DateSubm    string       `json:"Date_Subm"`
	JERecType   int          `json:"JE_Rec_Type"`
	DateEntd    string       `json:"Date_Entd"`
	AccCostCtr2 string       `json:"Acc_CostCtr2"`
	LastName    string       `json:"Last_Name"`
	AccNum      int          `json:"Acc_Num"`
	Field1      nulls.String `json:"Field1"`
	AccCostCtr1 string       `json:"Acc_CostCtr1"`
	PolicyID    int          `json:"Policy_ID"`
	Entity      string       `json:"Entity"`
	PolicyType  int          `json:"Policy_Type"`
	CustJE      float64      `json:"Cust_JE"`
	RMJE        float64      `json:"RM_JE"`
}

// userMap is a map of legacy ID to new ID
var userMap = map[int]uuid.UUID{}

// itemMap is a map of legacy ID to new ID
var itemMap = map[int]uuid.UUID{}

// itemCategoryMap is a map of legacy ID to new ID
var itemCategoryMap = map[int]uuid.UUID{}

// time used in place of missing time values
var (
	emptyTime    time.Time
	earliestTime time.Time
)

var _ = grift.Namespace("db", func() {
	_ = grift.Desc("import", "Import legacy data")
	_ = grift.Add("import", func(c *grift.Context) error {
		var obj LegacyData

		f, err := os.Open("./riskman.json")
		if err != nil {
			log.Fatal(err)
		}
		defer func(f *os.File) {
			if err := f.Close(); err != nil {
				panic("failed to close file, " + err.Error())
			}
		}(f)

		r := bufio.NewReader(f)
		dec := json.NewDecoder(r)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&obj); err != nil {
			return errors.New("json decode error: " + err.Error())
		}

		if err := models.DB.Transaction(func(tx *pop.Connection) error {
			fmt.Println("record counts: ")
			fmt.Printf("  Admin Users: %d\n", len(obj.Users))
			fmt.Printf("  Policies: %d\n", len(obj.Policies))
			fmt.Printf("  PolicyTypes: %d\n", len(obj.PolicyTypes))
			fmt.Printf("  Maintenance: %d\n", len(obj.Maintenance))
			fmt.Printf("  JournalEntries: %d\n", len(obj.JournalEntries))
			fmt.Printf("  ItemCategories: %d\n", len(obj.ItemCategories))
			fmt.Printf("  RiskCategories: %d\n", len(obj.RiskCategories))
			fmt.Printf("  LossReasons: %d\n", len(obj.LossReasons))
			fmt.Println("")

			importAdminUsers(tx, obj.Users)
			importItemCategories(tx, obj.ItemCategories)
			importPolicies(tx, obj.Policies)
			fmt.Print("Earliest time: ", earliestTime)

			return errors.New("blocking transaction commit until everything is ready")
		}); err != nil {
			log.Fatalf("failed to import, %s", err)
		}

		return nil
	})
})

func init() {
	emptyTime, _ = time.Parse(TimeFormat, EmptyTime)
	earliestTime, _ = time.Parse(TimeFormat, "2020-12-31 00:00:00")
	pop.Debug = false // Disable the Pop log messages
}

func importAdminUsers(tx *pop.Connection, in []LegacyUsers) {
	fmt.Println("Admin Users")
	fmt.Println("id,email,email_override,first_name,last_name,last_login_utc,location,staff_id,app_role")

	for _, user := range in {
		userID := stringToInt(user.Id, "User ID")
		userDesc := fmt.Sprintf("User[%d].", userID)

		newUser := models.User{
			Email:         user.Email,
			EmailOverride: user.EmailOverride,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			LastLoginUTC:  parseStringTime(user.LastLoginUtc, userDesc+"LastLoginUTC"),
			Location:      user.Location,
			StaffID:       user.StaffId,
			AppRole:       models.AppRoleAdmin,
			CreatedAt:     parseStringTime(user.CreatedAt, userDesc+"CreatedAt"),
			UpdatedAt:     parseStringTime(user.UpdatedAt, userDesc+"UpdatedAt"),
		}

		if err := newUser.Create(tx); err != nil {
			log.Fatalf("failed to create user, %s\n%+v", err, newUser)
		}

		userMap[userID] = newUser.ID

		fmt.Printf(`"%s","%s","%s","%s","%s","%s","%s","%s","%s"`+"\n",
			newUser.ID, newUser.Email, newUser.EmailOverride, newUser.FirstName, newUser.LastName,
			newUser.LastLoginUTC, newUser.Location, newUser.StaffID, newUser.AppRole,
		)
	}

	fmt.Println()
}

func importItemCategories(tx *pop.Connection, in []LegacyItemCategory) {
	fmt.Println("Item categories")
	fmt.Println("legacy_id,id,status,risk_category_id,name,auto_approve_max,help_text")

	for _, i := range in {
		categoryID := stringToInt(i.Id, "ItemCategory ID")

		newItemCategory := models.ItemCategory{
			RiskCategoryID: getRiskCategoryUUID(i.RiskCategoryId),
			Name:           i.Name,
			HelpText:       i.HelpText,
			Status:         getItemCategoryStatus(i),
			AutoApproveMax: fixedPointStringToInt(i.AutoApproveMax, "ItemCategory.AutoApproveMax"),
			CreatedAt:      parseStringTime(i.CreatedAt, fmt.Sprintf("ItemCategory[%d].CreatedAt", categoryID)),
			UpdatedAt:      parseStringTime(i.UpdatedAt, fmt.Sprintf("ItemCategory[%d].UpdatedAt", categoryID)),
			LegacyID:       nulls.NewInt(categoryID),
		}

		if err := newItemCategory.Create(tx); err != nil {
			log.Fatalf("failed to create item category, %s\n%+v", err, newItemCategory)
		}

		itemCategoryMap[categoryID] = newItemCategory.ID

		fmt.Printf(`%d,"%s","%s",%s,"%s",%d,"%s"`+"\n",
			newItemCategory.LegacyID.Int, newItemCategory.ID, newItemCategory.Status,
			newItemCategory.RiskCategoryID, newItemCategory.Name, newItemCategory.AutoApproveMax,
			newItemCategory.HelpText)
	}

	fmt.Println("")
}

func getRiskCategoryUUID(legacyID int) uuid.UUID {
	switch legacyID {
	case 1:
		return uuid.FromStringOrNil(models.RiskCategoryStationaryIDString)
	case 2, 3:
		return uuid.FromStringOrNil(models.RiskCategoryMobileIDString)
	}
	log.Printf("unrecognized risk category ID %d", legacyID)
	return uuid.FromStringOrNil(models.RiskCategoryMobileIDString)
}

func getItemCategoryStatus(itemCategory LegacyItemCategory) api.ItemCategoryStatus {
	var status api.ItemCategoryStatus

	// TODO: add other status values to this function

	switch itemCategory.Status {
	case "enabled":
		status = api.ItemCategoryStatusEnabled

	case "deprecated":
		status = api.ItemCategoryStatusDeprecated

	default:
		log.Printf("unrecognized item category status %s\n", itemCategory.Status)
		status = api.ItemCategoryStatus(itemCategory.Status)
	}

	return status
}

func importPolicies(tx *pop.Connection, in []LegacyPolicy) {
	nPolicies := 0
	nClaims := 0
	nItems := 0
	nClaimItems := 0

	for i := range in {
		normalizePolicy(&in[i])
		p := in[i]

		policyID := stringToInt(p.Id, "Policy ID")
		newPolicy := models.Policy{
			Type:        getPolicyType(p),
			HouseholdID: p.HouseholdId,
			CostCenter:  p.CostCenter,
			Account:     strconv.Itoa(p.Account),
			EntityCode:  p.EntityCode.String,
			LegacyID:    nulls.NewInt(policyID),
			CreatedAt:   parseStringTime(p.CreatedAt, fmt.Sprintf("Policy[%d].CreatedAt", policyID)),
			UpdatedAt:   parseNullStringTimeToTime(p.UpdatedAt, fmt.Sprintf("Policy[%d].UpdatedAt", policyID)),
		}
		if err := newPolicy.Create(tx); err != nil {
			log.Fatalf("failed to create policy, %s\n%+v", err, newPolicy)
		}
		nPolicies++

		importItems(tx, newPolicy, p.Items)
		nItems += len(p.Items)

		nClaimItems += importClaims(tx, newPolicy, p.Claims)
		nClaims += len(p.Claims)
	}

	fmt.Println("imported: ")
	fmt.Printf("  Policies: %d\n", nPolicies)
	fmt.Printf("  Claims: %d\n", nClaims)
	fmt.Printf("  Items: %d\n", nItems)
	fmt.Printf("  ClaimItems: %d\n", nClaimItems)
	fmt.Println("")
}

// getPolicyType gets the correct policy type
func getPolicyType(p LegacyPolicy) api.PolicyType {
	var policyType api.PolicyType

	switch p.Type {
	case "household":
		policyType = api.PolicyTypeHousehold
	case "ou", "corporate":
		policyType = api.PolicyTypeCorporate
	}

	return policyType
}

// normalizePolicy adjusts policy fields to pass validation checks
func normalizePolicy(p *LegacyPolicy) {
	if p.Type == "household" {
		p.CostCenter = ""
		p.EntityCode = nulls.String{}

		// TODO: fix input data so this isn't needed
		if p.HouseholdId == "" {
			log.Printf("Policy[%s].HouseholdId is empty\n", p.Id)
			p.HouseholdId = "-"
		}
	}
	if p.Type == "ou" || p.Type == "corporate" {
		p.HouseholdId = ""

		// TODO: fix input data so this isn't needed
		if !p.EntityCode.Valid || p.EntityCode.String == "" {
			log.Printf("Policy[%s].EntityCode is empty\n", p.Id)
			p.EntityCode = nulls.NewString("-")
		}
		if p.CostCenter == "" {
			log.Printf("Policy[%s].CostCenter is empty\n", p.Id)
			p.CostCenter = "-"
		}
	}
}

func importClaims(tx *pop.Connection, policy models.Policy, claims []LegacyClaim) int {
	nClaimItems := 0

	for _, c := range claims {
		claimID := stringToInt(c.Id, "Claim ID")
		claimDesc := fmt.Sprintf("Claim[%d].", claimID)
		newClaim := models.Claim{
			LegacyID:         nulls.NewInt(claimID),
			PolicyID:         policy.ID,
			EventDate:        parseStringTime(c.EventDate, claimDesc+"EventDate"),
			EventType:        getEventType(c),
			EventDescription: getEventDescription(c),
			Status:           getClaimStatus(c),
			ReviewDate:       nulls.NewTime(parseStringTime(c.ReviewDate, claimDesc+"ReviewDate")),
			// TODO: need user IDs
			// ReviewerID:       c.ReviewerId,
			PaymentDate: nulls.NewTime(parseStringTime(c.PaymentDate, claimDesc+"PaymentDate")),
			TotalPayout: fixedPointStringToInt(c.TotalPayout, "Claim.TotalPayout"),
			CreatedAt:   parseStringTime(c.CreatedAt, claimDesc+"CreatedAt"),
			UpdatedAt:   parseStringTime(c.UpdatedAt, claimDesc+"UpdatedAt"),
		}
		if err := newClaim.Create(tx); err != nil {
			log.Fatalf("failed to create claim, %s\n%+v", err, newClaim)
		}

		importClaimItems(tx, newClaim, c.ClaimItems)
		nClaimItems += len(c.ClaimItems)
	}

	return nClaimItems
}

func importClaimItems(tx *pop.Connection, claim models.Claim, items []LegacyClaimItem) {
	for _, c := range items {
		claimItemID := stringToInt(c.Id, "ClaimItem ID")
		itemDesc := fmt.Sprintf("ClaimItem[%d].", claimItemID)

		itemUUID, ok := itemMap[c.ItemId]
		if !ok {
			log.Fatalf("item ID %d not found in claim %d item list", claimItemID, claim.LegacyID.Int)
		}

		newClaimItem := models.ClaimItem{
			ClaimID:         claim.ID,
			ItemID:          itemUUID,
			Status:          getClaimItemStatus(c.Status),
			IsRepairable:    getIsRepairable(c),
			RepairEstimate:  fixedPointStringToInt(c.RepairEstimate, "ClaimItem.RepairEstimate"),
			RepairActual:    fixedPointStringToInt(c.RepairActual, "ClaimItem.RepairActual"),
			ReplaceEstimate: fixedPointStringToInt(c.ReplaceEstimate, "ClaimItem.ReplaceEstimate"),
			ReplaceActual:   fixedPointStringToInt(c.ReplaceActual, "ClaimItem.ReplaceActual"),
			PayoutOption:    c.PayoutOption,
			PayoutAmount:    fixedPointStringToInt(c.PayoutAmount, "ClaimItem.PayoutAmount"),
			FMV:             fixedPointStringToInt(c.Fmv, "ClaimItem.FMV"),
			ReviewDate:      parseStringTimeToNullTime(c.ReviewDate, itemDesc+"ReviewDate"),
			// ReviewerID:      c.ReviewerId, // TODO: get reviewer ID
			LegacyID:  claimItemID,
			CreatedAt: parseStringTime(c.CreatedAt, itemDesc+"CreatedAt"),
			UpdatedAt: parseStringTime(c.UpdatedAt, itemDesc+"UpdatedAt"),
		}

		if err := newClaimItem.Create(tx); err != nil {
			log.Fatalf("failed to create claim item %d, %s\nClaimItem:\n%+v", claimItemID, err, newClaimItem)
		}
	}
}

func getClaimItemStatus(status string) api.ClaimItemStatus {
	var s api.ClaimItemStatus

	switch status {
	case "pending":
		s = api.ClaimItemStatusPending
	case "revision":
		s = api.ClaimItemStatusRevision
	case "approved":
		s = api.ClaimItemStatusApproved
	case "denied":
		s = api.ClaimItemStatusDenied
	default:
		log.Printf("unrecognized claim item status: %s\n", status)
		s = api.ClaimItemStatus(status)
	}

	return s
}

func getIsRepairable(c LegacyClaimItem) bool {
	if c.IsRepairable != 0 && c.IsRepairable != 1 {
		log.Println("ClaimItem.IsRepairable is neither 0 nor 1")
	}
	return c.IsRepairable == 1
}

func getEventType(claim LegacyClaim) api.ClaimEventType {
	var eventType api.ClaimEventType

	// TODO: resolve "missing" types

	switch claim.EventType {
	case "Broken", "Dropped":
		eventType = api.ClaimEventTypeImpact
	case "Lightning", "Lightening":
		eventType = api.ClaimEventTypeElectricalSurge
	case "Theft":
		eventType = api.ClaimEventTypeTheft
	case "Water Damage":
		eventType = api.ClaimEventTypeWaterDamage
	case "Fire", "Miscellaneous", "Unknown", "Vandalism", "War":
		eventType = api.ClaimEventTypeOther
	default:
		log.Printf("unrecognized event type: %s\n", claim.EventType)
		eventType = api.ClaimEventTypeOther
	}

	return eventType
}

func getEventDescription(claim LegacyClaim) string {
	if claim.EventDescription == "" {
		// TODO: provide event descriptions on source data
		// log.Printf("missing event description on claim %s\n", claim.Id)
		return "-"
	}
	return claim.EventDescription
}

func getClaimStatus(claim LegacyClaim) api.ClaimStatus {
	var claimStatus api.ClaimStatus

	// TODO: add other status values to this function

	switch claim.Status {
	case "approved":
		claimStatus = api.ClaimStatusApproved

	default:
		log.Printf("unrecognized claim status %s\n", claim.Status)
		claimStatus = api.ClaimStatus(claim.Status)
	}

	return claimStatus
}

func importItems(tx *pop.Connection, policy models.Policy, items []LegacyItem) {
	for _, item := range items {
		itemID := stringToInt(item.Id, "Item ID")
		itemDesc := fmt.Sprintf("Item[%d].", itemID)

		newItem := models.Item{
			// TODO: name/policy needs to be unique
			Name:              item.Name + domain.GetUUID().String(),
			CategoryID:        itemCategoryMap[item.CategoryId],
			InStorage:         false,
			Country:           item.Country,
			Description:       item.Description,
			PolicyID:          policy.ID,
			Make:              item.Make,
			Model:             item.Model,
			SerialNumber:      item.SerialNumber,
			CoverageAmount:    fixedPointStringToInt(item.CoverageAmount, itemDesc+"CoverageAmount"),
			PurchaseDate:      parseStringTime(item.PurchaseDate, itemDesc+"PurchaseDate"),
			CoverageStatus:    getCoverageStatus(item),
			CoverageStartDate: parseStringTime(item.CoverageStartDate, itemDesc+"CoverageStartDate"),
			LegacyID:          nulls.NewInt(itemID),
			CreatedAt:         parseStringTime(item.CreatedAt, itemDesc+"CreatedAt"),
			UpdatedAt:         parseNullStringTimeToTime(item.UpdatedAt, itemDesc+"UpdatedAt"),
		}
		if err := newItem.Create(tx); err != nil {
			log.Fatalf("failed to create item, %s\n%+v", err, newItem)
		}
		itemMap[itemID] = newItem.ID
	}
}

func getCoverageStatus(item LegacyItem) api.ItemCoverageStatus {
	var coverageStatus api.ItemCoverageStatus

	switch item.CoverageStatus {
	case "approved":
		coverageStatus = api.ItemCoverageStatusApproved

	case "inactive":
		coverageStatus = api.ItemCoverageStatusInactive

	default:
		log.Printf("unknown coverage status %s\n", item.CoverageStatus)
		coverageStatus = api.ItemCoverageStatus(item.CoverageStatus)
	}

	return coverageStatus
}

func parseStringTime(t, desc string) time.Time {
	if t == "" {
		log.Printf("%s is empty, using %s", desc, EmptyTime)
		return emptyTime
	}
	tt, err := time.Parse(TimeFormat, t)
	if err != nil {
		log.Fatalf("failed to parse '%s' time '%s'", desc, t)
	}
	if tt.Before(earliestTime) {
		earliestTime = tt
	}
	return tt
}

func parseNullStringTimeToTime(t nulls.String, desc string) time.Time {
	var tt time.Time

	if !t.Valid {
		log.Printf("%s is null, using %s", desc, EmptyTime)
		return tt
	}

	var err error
	tt, err = time.Parse(TimeFormat, t.String)
	if err != nil {
		log.Fatalf("failed to parse '%s' time '%s'", desc, t.String)
	}

	if tt.Before(earliestTime) {
		earliestTime = tt
	}
	return tt
}

func parseStringTimeToNullTime(t, desc string) nulls.Time {
	if t == "" {
		log.Printf("time is empty, using null time, in %s", desc)
		return nulls.NewTime(emptyTime)
	}

	var tt time.Time
	var err error
	tt, err = time.Parse(TimeFormat, t)
	if err != nil {
		log.Fatalf("failed to parse '%s' time '%s'", desc, t)
	}

	if tt.Before(earliestTime) {
		earliestTime = tt
	}
	return nulls.NewTime(tt)
}

func stringToInt(s, msg string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("%s '%s' is not an int", msg, s)
	}
	return n
}

func fixedPointStringToInt(s, desc string) int {
	if s == "" {
		log.Printf("%s is empty", desc)
		return 0
	}

	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		log.Fatalf("%s has more than one '.' character: '%s'", desc, s)
	}
	intPart := stringToInt(parts[0], desc+" left of decimal")
	if len(parts[1]) != 2 {
		log.Fatalf("%s does not have two digits after the decimal: %s", desc, s)
	}
	fractionalPart := stringToInt(parts[1], desc+" right of decimal")
	return intPart*100 + fractionalPart
}
