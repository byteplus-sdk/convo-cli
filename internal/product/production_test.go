// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package product

import (
	"reflect"
	"testing"
)

func TestProductionProfileIsolation(t *testing.T) {
	if got := RegisteredIDs(); !reflect.DeepEqual(got, []string{BytePlusID}) {
		t.Fatalf("production profiles = %v", got)
	}
	p := Current()
	if p.ID != BytePlusID || p.BinaryName != "convo-cli" || p.ConfigFileName != "byteplus-rtc.config.yaml" || p.SDKName != "@byteplus/rtc" || p.SDKVersion != "4.69.3" {
		t.Fatalf("production identity = %+v", p)
	}
	if DefaultOpenAPIEndpoint != p.OpenAPIEndpoint || DefaultOpenAPIRegion != p.OpenAPIRegion || DefaultVoiceChatVersion != p.VoiceChatVersion || DefaultAuthorizeURL != p.OAuthAuthorizeEndpoint || DefaultTokenURL != p.OAuthTokenEndpoint || DefaultDocsMCPEndpoint != p.DocsMCPEndpoint {
		t.Fatal("production defaults differ from the selected profile")
	}
	if p.AuthHomeEnv != "CONVO_CLI_HOME" || p.AuthDirName != ".convo-cli" || p.OfficialSkillName != "byted-bp-convo-guide" {
		t.Fatalf("production local state = %+v", p)
	}
}
