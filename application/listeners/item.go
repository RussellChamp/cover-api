package listeners

import (
	"fmt"

	"github.com/gobuffalo/events"

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
	"github.com/silinternational/cover-api/messages"
	"github.com/silinternational/cover-api/models"
	"github.com/silinternational/cover-api/notifications"
)

const wrongStatusMsg = "error with %s listener. Object has wrong status: %s"

func addMessageItemData(msg *notifications.Message, item models.Item) {
	msg.Data["itemURL"] = fmt.Sprintf("%s/items/%s", domain.Env.UIURL, item.ID)
	msg.Data["itemName"] = item.Name
	return
}

func newItemMessageForMember(item models.Item, member models.User) notifications.Message {
	msg := notifications.NewEmailMessage()
	addMessageItemData(&msg, item)
	msg.ToName = member.Name()
	msg.ToEmail = member.EmailOfChoice()
	msg.Data["memberName"] = member.Name()

	return msg
}

func itemSubmitted(e events.Event) {
	var item models.Item
	if err := findObject(e.Payload, &item, e.Kind); err != nil {
		return
	}

	if item.CoverageStatus == api.ItemCoverageStatusApproved {
		// TODO any business rules to deal with here
	} else if item.CoverageStatus == api.ItemCoverageStatusPending { // Was submitted but not auto approved
		// TODO any business rules to deal with here
	} else {
		domain.ErrLogger.Printf(wrongStatusMsg, "itemSubmitted", item.CoverageStatus)
	}

	messages.ItemSubmittedSend(item, getNotifiersFromEventPayload(e.Payload))
}

func notifyItemApprovedMember(item models.Item, notifiers []interface{}) {
	for _, m := range item.Policy.Members {
		msg := newItemMessageForMember(item, m)
		msg.Template = domain.MessageTemplateItemApprovedMember
		msg.Subject = "your new policy item has been approved"
		if err := notifications.Send(msg, notifiers...); err != nil {
			domain.ErrLogger.Printf("error sending item auto approved notification to member, %s", err)
		}
	}
}

func itemRevision(e events.Event) {
	var item models.Item
	if err := findObject(e.Payload, &item, e.Kind); err != nil {
		return
	}

	if item.CoverageStatus != api.ItemCoverageStatusRevision {
		panic(fmt.Sprintf(wrongStatusMsg, "itemRevision", item.CoverageStatus))
	}

	item.LoadPolicyMembers(models.DB, false)
	notifiers := getNotifiersFromEventPayload(e.Payload)

	// TODO figure out how to specify required revisions

	for _, m := range item.Policy.Members {
		msg := newItemMessageForMember(item, m)
		msg.Template = domain.MessageTemplateItemRevisionMember
		msg.Subject = "changes have been requested on your new policy item"
		if err := notifications.Send(msg, notifiers...); err != nil {
			domain.ErrLogger.Printf("error sending item revision notification to member, %s", err)
		}
	}
}

func itemApproved(e events.Event) {
	var item models.Item
	if err := findObject(e.Payload, &item, e.Kind); err != nil {
		return
	}

	if item.CoverageStatus != api.ItemCoverageStatusApproved {
		domain.ErrLogger.Printf(wrongStatusMsg, "itemApproved", item.CoverageStatus)
		return
	}

	item.LoadPolicyMembers(models.DB, false)

	notifiers := getNotifiersFromEventPayload(e.Payload)
	notifyItemApprovedMember(item, notifiers)
	// TODO do whatever else needs doing
}

func itemDenied(e events.Event) {
	var item models.Item
	if err := findObject(e.Payload, &item, e.Kind); err != nil {
		return
	}

	if item.CoverageStatus != api.ItemCoverageStatusDenied {
		domain.ErrLogger.Printf(wrongStatusMsg, "itemDenied", item.CoverageStatus)
		return
	}

	item.LoadPolicyMembers(models.DB, false)
	notifiers := getNotifiersFromEventPayload(e.Payload)

	// TODO figure out how to give a reason for the denial

	for _, m := range item.Policy.Members {
		msg := newItemMessageForMember(item, m)
		msg.Template = domain.MessageTemplateItemDeniedMember
		msg.Subject = "coverage on your new policy item has been denied"
		if err := notifications.Send(msg, notifiers...); err != nil {
			domain.ErrLogger.Printf("error sending item denied notification to member, %s", err)
		}
	}
}
