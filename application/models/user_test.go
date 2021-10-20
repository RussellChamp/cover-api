package models

import (
	"testing"

	"github.com/gofrs/uuid"
)

func (ms *ModelSuite) TestUser_Validate() {
	t := ms.T()
	tests := []struct {
		name     string
		user     User
		wantErr  bool
		errField string
	}{
		{
			name: "minimum",
			user: User{
				Email:   "user@example.com",
				AppRole: AppRoleUser,
			},
			wantErr: false,
		},
		{
			name: "missing email",
			user: User{
				AppRole: AppRoleUser,
			},
			wantErr:  true,
			errField: "User.Email",
		},
		{
			name: "missing approle",
			user: User{
				Email: "dummy@dusos.com",
			},
			wantErr:  true,
			errField: "User.AppRole",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vErr, _ := tt.user.Validate(DB)
			if tt.wantErr {
				if vErr.Count() == 0 {
					t.Errorf("Expected an error, but did not get one")
				} else if len(vErr.Get(tt.errField)) == 0 {
					t.Errorf("Expected an error on field %v, but got none (errors: %+v)", tt.errField, vErr.Errors)
				}
			} else if vErr.HasAny() {
				t.Errorf("Unexpected error: %+v", vErr)
			}
		})
	}
}

func (ms *ModelSuite) TestUser_CreateInitialPolicy() {
	t := ms.T()

	pf := CreatePolicyFixtures(ms.DB, FixturesConfig{NumberOfPolicies: 1})
	policy := pf.Policies[0]

	uf := CreateUserFixtures(ms.DB, 2)
	userNoPolicy := uf.Users[0]

	userWithPolicy := uf.Users[1]

	pUser := PolicyUser{
		PolicyID: policy.ID,
		UserID:   userWithPolicy.ID,
	}

	ms.NoError(pUser.Create(ms.DB))

	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name:    "missing ID",
			user:    User{},
			wantErr: true,
		},
		{
			name:    "policy to be created",
			user:    userNoPolicy,
			wantErr: false,
		},
		{
			name:    "no new policy to create",
			user:    userWithPolicy,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.user.CreateInitialPolicy(DB)
			if tt.wantErr {
				ms.Error(err)
				return
			}

			ms.NoError(err)

			policyUsers := PolicyUsers{}
			err = ms.DB.Where("user_id = ?", tt.user.ID).All(&policyUsers)
			ms.NoError(err, "error trying to find resulting policyUsers")
			ms.Len(policyUsers, 1, "incorrect number of policyUsers")
		})
	}
}

func (ms *ModelSuite) TestUser_FindStewards() {
	CreateUserFixtures(ms.DB, 3)
	steward0 := CreateAdminUsers(ms.DB)[AppRoleSteward]
	steward1 := CreateAdminUsers(ms.DB)[AppRoleSteward]

	var users Users
	users.FindStewards(ms.DB)
	want := map[uuid.UUID]bool{steward0.ID: true, steward1.ID: true}

	got := map[uuid.UUID]bool{}
	for _, s := range users {
		got[s.ID] = true
	}

	ms.EqualValues(want, got, "incorrect steward ids")
}

func (ms *ModelSuite) TestUser_FindSignators() {
	CreateUserFixtures(ms.DB, 3)
	signator0 := CreateAdminUsers(ms.DB)[AppRoleSignator]
	signator1 := CreateAdminUsers(ms.DB)[AppRoleSignator]

	var users Users
	users.FindSignators(ms.DB)
	want := map[uuid.UUID]bool{signator0.ID: true, signator1.ID: true}

	got := map[uuid.UUID]bool{}
	for _, s := range users {
		got[s.ID] = true
	}

	ms.EqualValues(want, got, "incorrect signator ids")
}

func (ms *ModelSuite) TestUser_EmailOfChoice() {
	justEmail := User{Email: "justemail@example.com"}
	hasOverride := User{Email: "main@example.com", EmailOverride: "override@example.com"}

	got := justEmail.EmailOfChoice()
	ms.Equal(justEmail.Email, got, "incorrect Email for user with no override email")

	got = hasOverride.EmailOfChoice()
	ms.Equal(hasOverride.EmailOverride, got, "incorrect Email for user with an override email")
}

func (ms *ModelSuite) TestUser_Name() {
	t := ms.T()
	tests := []struct {
		name string
		user User
		want string
	}{
		{
			name: "only first",
			user: User{FirstName: "  OnlyFirst "},
			want: "OnlyFirst",
		},
		{
			name: "only last",
			user: User{LastName: "  OnlyLast "},
			want: "OnlyLast",
		},
		{
			name: "no extra spaces",
			user: User{FirstName: "First", LastName: "Last"},
			want: "First Last",
		},
		{
			name: "has extra spaces",
			user: User{FirstName: "  First  ", LastName: "  Last  "},
			want: "First Last",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.user.Name()
			ms.Equal(tt.want, got, "incorrect user name")
		})
	}
}

func (ms *ModelSuite) TestUser_OwnsFile() {
	userFixtures := CreateUserFixtures(ms.DB, 2)
	userOwnsFile := userFixtures.Users[1]
	userNoFile := userFixtures.Users[0]

	fileFixtures := CreateFileFixtures(ms.DB, 1, userOwnsFile.ID)
	file := fileFixtures.Files[0]

	tests := []struct {
		name    string
		user    User
		file    File
		want    bool
		wantErr bool
	}{
		{
			name:    "user not valid",
			file:    file,
			wantErr: true,
		},
		{
			name:    "not owned",
			user:    userNoFile,
			file:    file,
			want:    false,
			wantErr: false,
		},
		{
			name:    "owned",
			user:    userOwnsFile,
			file:    file,
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			ownsFile, err := tt.user.OwnsFile(ms.DB, tt.file)
			if tt.wantErr {
				ms.Error(err)
				return
			}
			ms.NoError(err)
			ms.Equal(tt.want, ownsFile, "incorrect result from OwnsFile")
		})
	}
}
