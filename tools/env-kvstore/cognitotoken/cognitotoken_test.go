package cognitotoken

import (
	"testing"
)

var (
	jwt_with_session_naming_claims    = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJ0ZXN0IiwiaWF0IjpudWxsLCJleHAiOm51bGwsImF1ZCI6Im5vbmUiLCJzdWIiOiJ0ZXN0IiwicnVuX2lkIjoicnVuSUQiLCJzaGEiOiJTSEEifQ.1w2YtkjMCMOeyIwoRKhB3keJkF9AZh2CmdcYD6EFcwg"
	jwt_without_session_naming_claims = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJ0ZXN0IiwiaWF0IjpudWxsLCJleHAiOm51bGwsImF1ZCI6Im5vbmUiLCJzdWIiOiJ0ZXN0Iiwibm90X3J1bl9pZCI6InJ1bklEIiwibm90X3NoYSI6IlNIQSJ9.r4cCaI_W09e-W2dxzdg5uwjH3rlCcI5jFBb4Bugm6Uw"
)

func TestGetAWSSessionName(t *testing.T) {
	tests := []struct {
		name           string
		exchanger      *CognitoGHATokenExchanger
		expectedName   string
		expectingError bool
	}{
		{
			name: "JWT with session naming claims",
			exchanger: &CognitoGHATokenExchanger{
				ghaJWT: jwt_with_session_naming_claims,
			},
			expectedName:   "runID@SHA",
			expectingError: false,
		},
		{
			name: "JWT without session naming claims",
			exchanger: &CognitoGHATokenExchanger{
				ghaJWT: jwt_without_session_naming_claims,
			},
			expectedName:   "",
			expectingError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionName, err := tt.exchanger.getAWSSessionName()
			if tt.expectingError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectingError && err != nil {
				t.Errorf("did not expect error but got: %v", err)
			}
			if sessionName != tt.expectedName {
				t.Errorf("expected session name '%s', got '%s'", tt.expectedName, sessionName)
			}
		})
	}
}
