package actions

import (
	"errors"
	"net/http"

	"github.com/gobuffalo/buffalo"

	"github.com/silinternational/riskman-api/api"
	"github.com/silinternational/riskman-api/domain"
	"github.com/silinternational/riskman-api/models"
)

// swagger:operation GET /claims Claims ClaimsList
//
// ClaimsList
//
// list all the current user's claims, or all Claims if called as an admin
//
// ---
// responses:
//   '200':
//     description: a list of Claims
//     schema:
//       type: array
//       items:
//         "$ref": "#/definitions/Claim"
func claimsList(c buffalo.Context) error {
	tx := models.Tx(c)

	// TODO: list only current user's claims
	var claims models.Claims
	if err := tx.All(&claims); err != nil {
		return c.Render(http.StatusInternalServerError, r.JSON(err))
	}

	return renderOk(c, models.ConvertClaims(claims))
}

// swagger:operation GET /claims/{id} Claims ClaimsView
//
// ClaimsView
//
// view a specific claim
//
// ---
// responses:
//   '200':
//     description: a Claim
//     schema:
//       "$ref": "#/definitions/Claim"
func claimsView(c buffalo.Context) error {
	claim := getReferencedClaimFromCtx(c)
	if claim == nil {
		err := errors.New("claim not found in context")
		return reportError(c, api.NewAppError(err, "", api.CategoryInternal))
	}
	return renderOk(c, models.ConvertClaim(*claim))
}

// swagger:operation POST /policy/{id}/claims Claims ClaimsCreate
//
// ClaimsCreate
//
// create a new Claim on a policy
//
// ---
// responses:
//   '200':
//     description: the new Claim
//     schema:
//       "$ref": "#/definitions/Claim"
func claimsCreate(c buffalo.Context) error {
	policy := getReferencedPolicyFromCtx(c)
	if policy == nil {
		err := errors.New("policy not found in route")
		return reportError(c, api.NewAppError(err, api.ErrorPolicyNotFound, api.CategoryUser))
	}

	var input api.ClaimCreateInput
	if err := StrictBind(c, &input); err != nil {
		return reportError(c, api.NewAppError(err, api.ErrorClaimCreateInvalidInput, api.CategoryUser))
	}

	tx := models.Tx(c)
	if err := policy.AddClaim(tx, input); err != nil {
		return reportError(c, err)
	}

	return renderOk(c, models.ConvertPolicy(tx, *policy))
}

// getReferencedClaimFromCtx pulls the models.Claim resource from context that was put there
// by the AuthZ middleware
func getReferencedClaimFromCtx(c buffalo.Context) *models.Claim {
	claim, ok := c.Value(domain.TypeClaim).(*models.Claim)
	if !ok {
		return nil
	}
	return claim
}
