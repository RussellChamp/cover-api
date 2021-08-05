package actions

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gobuffalo/buffalo"
	"github.com/gobuffalo/buffalo/render"
	"github.com/gobuffalo/nulls"

	"github.com/silinternational/riskman-api/api"
	"github.com/silinternational/riskman-api/auth"
	"github.com/silinternational/riskman-api/auth/saml"
	"github.com/silinternational/riskman-api/domain"
	"github.com/silinternational/riskman-api/models"
)

const (
	// http param for access token
	AccessTokenParam = "access-token"

	// http param and session key for Client ID
	ClientIDParam      = "client-id"
	ClientIDSessionKey = "ClientID"

	// logout http param for what is normally the bearer token
	LogoutToken = "token"

	// http param and session key for ReturnTo
	ReturnToParam      = "return-to"
	ReturnToSessionKey = "ReturnTo"

	// http param for token type
	TokenTypeParam = "token-type"
)

var samlConfig = saml.Config{
	IDPEntityID:                 domain.Env.SamlIdpEntityId,
	SPEntityID:                  domain.Env.SamlSpEntityId,
	SingleSignOnURL:             domain.Env.SamlSsoURL,
	SingleLogoutURL:             domain.Env.SamlSloURL,
	AudienceURI:                 domain.Env.SamlAudienceUri,
	AssertionConsumerServiceURL: domain.Env.SamlAssertionConsumerServiceUrl,
	IDPPublicCert:               replaceNewLines(domain.Env.SamlIdpCert),
	SPPublicCert:                replaceNewLines(domain.Env.SamlSpCert),
	SPPrivateKey:                replaceNewLines(domain.Env.SamlSpPrivateKey),
	SignRequest:                 domain.Env.SamlSignRequest,
	CheckResponseSigning:        domain.Env.SamlCheckResponseSigning,
	AttributeMap:                nil,
}

func setCurrentUser(next buffalo.Handler) buffalo.Handler {
	return func(c buffalo.Context) error {
		bearerToken := domain.GetBearerTokenFromRequest(c.Request())
		if bearerToken == "" {
			err := errors.New("no bearer token provided")
			return reportError(c, api.NewAppError(err, api.ErrorNotAuthorized, api.CategoryUnauthorized))
		}

		var userAccessToken models.UserAccessToken
		tx := models.Tx(c)
		err := userAccessToken.FindByBearerToken(tx, bearerToken)
		if err != nil {
			if domain.IsOtherThanNoRows(err) {
				domain.Error(c, err.Error())
			}
			return reportError(c, &api.AppError{
				HttpStatus: http.StatusUnauthorized,
				Key:        api.ErrorNotAuthorized,
				DebugMsg:   "invalid bearer token",
			})
		}

		isExpired, err := userAccessToken.DeleteIfExpired(tx)
		if err != nil {
			domain.Error(c, err.Error())
		}

		if isExpired {
			return reportError(c, &api.AppError{
				HttpStatus: http.StatusUnauthorized,
				Key:        api.ErrorNotAuthorized,
				DebugMsg:   "expired bearer token",
			})
		}

		user, err := userAccessToken.GetUser(tx)
		if err != nil {
			newExtra(c, "tokenID", userAccessToken.ID)
			return reportError(c, &api.AppError{
				HttpStatus: http.StatusInternalServerError,
				Key:        api.ErrorQueryFailure,
				DebugMsg:   "error finding user by access token, " + err.Error(),
			})
		}

		userAccessToken.LastUsedAt = nulls.NewTime(time.Now().UTC())
		if err := userAccessToken.Save(tx); err != nil {
			domain.Error(c, "error saving userAccessToken with new LastUsedAt value: "+err.Error(), nil)
		}

		c.Set(domain.ContextKeyCurrentUser, user)

		// set person on rollbar session
		domain.RollbarSetPerson(c, user.ID.String(), user.FirstName, user.LastName, user.Email)
		msg := fmt.Sprintf("user %s authenticated with bearer token from ip %s", user.Email, c.Request().RemoteAddr)
		domain.NewExtra(c, "user_id", user.ID)
		domain.NewExtra(c, "email", user.Email)
		domain.NewExtra(c, "ip", c.Request().RemoteAddr)
		domain.Info(c, msg)

		return next(c)
	}
}

func authRequest(c buffalo.Context) error {
	// Push the Client ID into the Session
	clientID := c.Param(ClientIDParam)
	if clientID == "" {
		appErr := api.AppError{
			HttpStatus: http.StatusBadRequest,
			Key:        api.ErrorMissingClientID,
			Message:    ClientIDParam + " is required to login",
		}
		return reportErrorAndClearSession(c, &appErr)
	}

	c.Session().Set(ClientIDSessionKey, clientID)

	getOrSetReturnTo(c)

	sp, err := saml.New(samlConfig)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorLoadingAuthProvider,
			Message:    "unable to load saml auth provider.",
		})
	}

	redirectURL, err := sp.AuthRequest(c)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorGettingAuthURL,
			Message:    "unable to determine what the saml authentication url should be",
		})
	}

	authRedirect := map[string]string{
		"RedirectURL": redirectURL,
	}

	// Reply with a 200 and leave it to the UI to do the redirect
	return c.Render(http.StatusOK, render.JSON(authRedirect))
}

func getOrSetReturnTo(c buffalo.Context) string {
	returnTo := c.Param(ReturnToParam)

	if returnTo == "" {
		var ok bool
		returnTo, ok = c.Session().Get(ReturnToSessionKey).(string)
		if !ok {
			returnTo = domain.DefaultUIPath
		}

		return returnTo
	}

	c.Session().Set(ReturnToSessionKey, returnTo)

	return returnTo
}

func authCallback(c buffalo.Context) error {
	clientID, ok := c.Session().Get(ClientIDSessionKey).(string)
	if !ok {
		appError := api.AppError{
			Key:        api.ErrorMissingSessionKey,
			DebugMsg:   ClientIDSessionKey + " session entry is required to complete login",
			HttpStatus: http.StatusFound,
		}
		return reportErrorAndClearSession(c, &appError)
	}

	sp, err := saml.New(samlConfig)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorLoadingAuthProvider,
			Message:    "unable to load saml auth provider in auth callback.",
		})
	}

	authResp := sp.AuthCallback(c)
	if authResp.Error != nil {
		reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorAuthProvidersCallback,
			Message:    authResp.Error.Error(),
		})
	}

	returnTo := getOrSetReturnTo(c)

	if authResp.AuthUser == nil {
		reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusFound,
			Key:        api.ErrorAuthProvidersCallback,
			Message:    "nil authResp.AuthUser",
		})
	}

	// if we have an authuser, find or create user in local db and finish login
	var user models.User

	// login was success, clear session so new login can be initiated if needed
	c.Session().Clear()

	authUser := authResp.AuthUser
	tx := models.Tx(c)
	if err := user.FindOrCreateFromAuthUser(tx, authUser); err != nil {
		reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorWithAuthUser,
			Message:    err.Error(),
		})
	}

	isNew := false
	if time.Since(user.CreatedAt) < time.Duration(time.Second*30) {
		isNew = true
	}
	authUser.IsNew = isNew

	uat, err := user.CreateAccessToken(tx, clientID)
	if err != nil {
		reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorCreatingAccessToken,
			Message:    err.Error(),
		})
	}

	authUser.AccessToken = uat.AccessToken
	authUser.AccessTokenExpiresAt = uat.ExpiresAt.UTC().Unix()

	// set person on rollbar session
	domain.RollbarSetPerson(c, user.StaffID, user.FirstName, user.LastName, user.Email)

	return c.Redirect(302, getLoginSuccessRedirectURL(*authUser, returnTo))
}

// getLoginSuccessRedirectURL generates the URL for redirection after a successful login
func getLoginSuccessRedirectURL(authUser auth.User, returnTo string) string {
	uiURL := domain.Env.UIURL

	params := fmt.Sprintf("?%s=Bearer&%s=%s",
		TokenTypeParam, AccessTokenParam, authUser.AccessToken)

	// New Users go straight to the welcome page
	if authUser.IsNew {
		uiURL += "/welcome"
		if len(returnTo) > 0 {
			params += "&" + ReturnToParam + "=" + url.QueryEscape(returnTo)
		}
		return uiURL + params
	}

	// Avoid two question marks in the params
	if strings.Contains(returnTo, "?") && strings.HasPrefix(params, "?") {
		params = "&" + params[1:]
	}

	return uiURL + returnTo + params
}

// authDestroy uses the bearer token to find the user's access token and
//  calls the appropriate provider's logout function.
func authDestroy(c buffalo.Context) error {
	tokenParam := c.Param(LogoutToken)
	if tokenParam == "" {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorCreatingAccessToken,
			Message:    LogoutToken + " is required to logout",
		})
	}

	var uat models.UserAccessToken
	tx := models.Tx(c)
	err := uat.FindByBearerToken(tx, tokenParam)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorFindingAccessToken,
			Message:    err.Error(),
		})
	}

	authUser, err := uat.GetUser(tx)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorAuthProvidersLogout,
			Message:    err.Error(),
		})
	}

	// set person on rollbar session
	domain.RollbarSetPerson(c, authUser.ID.String(), authUser.FirstName, authUser.LastName, authUser.Email)

	sp, err := saml.New(samlConfig)
	if err != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorLoadingAuthProvider,
			Message:    err.Error(),
		})
	}

	authResp := sp.Logout(c)
	if authResp.Error != nil {
		return reportErrorAndClearSession(c, &api.AppError{
			HttpStatus: http.StatusInternalServerError,
			Key:        api.ErrorAuthProvidersLogout,
			Message:    authResp.Error.Error(),
		})
	}

	redirectURL := domain.Env.UIURL

	if authResp.RedirectURL != "" {
		var uat models.UserAccessToken
		err = uat.DeleteByBearerToken(tx, tokenParam)
		if err != nil {
			return reportErrorAndClearSession(c, &api.AppError{
				HttpStatus: http.StatusInternalServerError,
				Key:        api.ErrorDeletingAccessToken,
				Message:    err.Error(),
			})
		}
		c.Session().Clear()
		redirectURL = authResp.RedirectURL
	}

	return c.Redirect(http.StatusFound, redirectURL)
}

func replaceNewLines(input string) string {
	return strings.Replace(input, `\n`, "\n", -1)
}

func checkSamlConfig() {
	if domain.Env.GoEnv == "development" || domain.Env.GoEnv == "test" {
		return
	}
	if domain.Env.SamlIdpEntityId == "" {
		panic("required SAML variable SamlIdpEntityId is undefined")
	}
	if domain.Env.SamlSpEntityId == "" {
		panic("required SAML variable SamlSpEntityId is undefined")
	}
	if domain.Env.SamlSsoURL == "" {
		panic("required SAML variable SamlSsoURL is undefined")
	}
	if domain.Env.SamlSloURL == "" {
		panic("required SAML variable SamlSloURL is undefined")
	}
	if domain.Env.SamlAudienceUri == "" {
		panic("required SAML variable SamlAudienceUri is undefined")
	}
	if domain.Env.SamlAssertionConsumerServiceUrl == "" {
		panic("required SAML variable SamlAssertionConsumerServiceUrl is undefined")
	}
	if domain.Env.SamlIdpCert == "" {
		panic("required SAML variable SamlIdpCert is undefined")
	}
	if domain.Env.SamlSpCert == "" {
		panic("required SAML variable SamlSpCert is undefined")
	}
	if domain.Env.SamlSpPrivateKey == "" {
		panic("required SAML variable SamlSpPrivateKey is undefined")
	}
}
