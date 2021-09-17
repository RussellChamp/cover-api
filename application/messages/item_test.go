package messages

import (
	"testing"

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
	"github.com/silinternational/cover-api/models"
	"github.com/silinternational/cover-api/notifications"
)

func (ts *TestSuite) Test_ItemSubmittedQueueMessage() {
	t := ts.T()
	db := ts.DB

	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    1,
		UsersPerPolicy:      2,
		ClaimsPerPolicy:     1,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
	}

	f := models.CreateItemFixtures(db, fixConfig)

	steward0 := models.CreateAdminUsers(db)[models.AppRoleSteward]
	steward1 := models.CreateAdminUsers(db)[models.AppRoleSteward]
	member0 := f.Policies[0].Members[0]
	member1 := f.Policies[0].Members[1]

	submittedItem := models.UpdateItemStatus(db, f.Items[0], api.ItemCoverageStatusPending, "")
	approvedItem := models.UpdateItemStatus(db, f.Items[1], api.ItemCoverageStatusApproved, "")

	testEmailer := notifications.DummyEmailService{}

	tests := []struct {
		data testData
		item models.Item
	}{
		{
			data: testData{
				name:                  "just submitted, not approved",
				wantToEmails:          []interface{}{steward0.EmailOfChoice(), steward1.EmailOfChoice()},
				wantSubjectContains:   "just submitted a new policy item for approval",
				wantInappTextContains: "A new policy item is waiting for your approval",
				wantBodyContains: []string{
					domain.Env.UIURL,
					submittedItem.Name,
					"just submitted a policy item which needs your attention",
				},
			},
			item: submittedItem,
		},
		{
			data: testData{
				name:                  "auto approved - members",
				wantToEmails:          []interface{}{member0.EmailOfChoice(), member1.EmailOfChoice()},
				wantSubjectContains:   "your new policy item has been approved",
				wantInappTextContains: "your new policy item has been approved",
				wantBodyContains: []string{
					domain.Env.UIURL,
					approvedItem.Name,
					"Your newly submitted policy item has been approved.",
				},
			},
			item: approvedItem,
		},
		{
			data: testData{
				name:                  "auto approved - stewards",
				wantToEmails:          []interface{}{steward0.EmailOfChoice(), steward1.EmailOfChoice()},
				wantSubjectContains:   "a new policy item that has been auto approved",
				wantInappTextContains: "Coverage on a new policy item was just auto approved",
				wantBodyContains: []string{
					domain.Env.UIURL,
					approvedItem.Name,
					"just submitted a policy item which has been auto approved.",
				},
			},
			item: approvedItem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.data.name, func(t *testing.T) {
			testEmailer.DeleteSentMessages()
			ItemSubmittedQueueMessage(db, tt.item)
			validateNotificationUsers(ts, db, tt.data)

			notfns := models.Notifications{}
			ts.NoError(db.All(&notfns), "error fetching all NotificationUsers for destroy")
			ts.NoError(db.Destroy(&notfns), "error destroying all NotificationUsers")
		})
	}
}

func (ts *TestSuite) Test_ItemRevisionQueueMessage() {
	t := ts.T()
	db := ts.DB

	fixConfig := models.FixturesConfig{
		NumberOfPolicies: 1,
		UsersPerPolicy:   2,
		ItemsPerPolicy:   2,
	}

	f := models.CreateItemFixtures(db, fixConfig)

	member0 := f.Policies[0].Members[0]
	member1 := f.Policies[0].Members[1]

	revisionItem := f.Items[0]
	models.UpdateItemStatus(db, revisionItem, api.ItemCoverageStatusRevision, "you can't be serious")

	tests := []testData{
		{
			name:                  "revisions required",
			wantToEmails:          []interface{}{member0.EmailOfChoice(), member1.EmailOfChoice()},
			wantSubjectContains:   "changes have been requested on your new policy item",
			wantInappTextContains: "changes have been requested on your new policy item",
			wantBodyContains: []string{
				domain.Env.UIURL,
				revisionItem.Name,
				"revisions have been requested",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ItemRevisionQueueMessage(db, revisionItem)
			var notnUsers models.NotificationUsers
			ts.NoError(db.Where("email_address in (?)",
				tt.wantToEmails[0], tt.wantToEmails[1]).All(&notnUsers))

			validateNotificationUsers(ts, db, tt)
		})
	}
}

func (ts *TestSuite) Test_ItemDeniedQueueMessage() {
	t := ts.T()
	db := ts.DB

	fixConfig := models.FixturesConfig{
		NumberOfPolicies: 1,
		UsersPerPolicy:   2,
		ItemsPerPolicy:   2,
	}

	f := models.CreateItemFixtures(db, fixConfig)

	member0 := f.Policies[0].Members[0]
	member1 := f.Policies[0].Members[1]

	deniedItem := f.Items[0]
	models.UpdateItemStatus(db, deniedItem, api.ItemCoverageStatusDenied, "this will never fly")

	tests := []testData{
		{
			name:                  "coverage denied",
			wantToEmails:          []interface{}{member0.EmailOfChoice(), member1.EmailOfChoice()},
			wantSubjectContains:   "coverage on your new policy item has been denied",
			wantInappTextContains: "coverage on your new policy item has been denied",
			wantBodyContains: []string{
				domain.Env.UIURL,
				deniedItem.Name,
				"Coverage on your newly submitted policy item has been denied.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ItemDeniedQueueMessage(db, deniedItem)
			validateNotificationUsers(ts, db, tt)
		})
	}
}
