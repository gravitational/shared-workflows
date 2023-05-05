package github

import (
	"encoding/json"
	"testing"

	go_github "github.com/google/go-github/v37/github"
)

func TestFindTreeBlobEntries(t *testing.T) {
	var tree *go_github.Tree
	treeJSON := `{
  "sha": "95d933bddfb5bb9e7b0eeb8fcafb453b9c5ed1d0",
  "tree": [
    {
      "sha": "77ba35c445776f2e076c642cab490bedc85d3282",
      "path": ".drone.yml",
      "mode": "100644",
      "type": "blob",
      "size": 5307,
      "url": "https://api.github.com/repos/gravitational/cloud/git/blobs/77ba35c445776f2e076c642cab490bedc85d3282"
    },
    {
      "sha": "862914f5917e2cc713da1147404c53f4102289e9",
      "path": ".github",
      "mode": "040000",
      "type": "tree",
      "url": "https://api.github.com/repos/gravitational/cloud/git/trees/862914f5917e2cc713da1147404c53f4102289e9"
    },
    {
      "sha": "e7b66702a51d900f0185290d413d6076984b1735",
      "path": "db/salescenter/migrations/202301031000_product-alter.up.sql",
      "mode": "100644",
      "type": "blob",
      "size": 87,
      "url": "https://api.github.com/repos/gravitational/cloud/git/blobs/e7b66702a51d900f0185290d413d6076984b1735"
    },
    {
      "sha": "ffb28ae64f4f4fc575b2abd236e22a05f7f57c07",
      "path": ".github/ISSUE_TEMPLATE",
      "mode": "040000",
      "type": "tree",
      "url": "https://api.github.com/repos/gravitational/cloud/git/trees/ffb28ae64f4f4fc575b2abd236e22a05f7f57c07"
    },
    {
      "sha": "627021ef0ccdc45dd52be3da49cf311d429cffa5",
      "path": ".github/ISSUE_TEMPLATE/tenant_upgrade.md",
      "mode": "100644",
      "type": "blob",
      "size": 2665,
      "url": "https://api.github.com/repos/gravitational/cloud/git/blobs/627021ef0ccdc45dd52be3da49cf311d429cffa5"
    },
    {
      "sha": "293bbd2c51c1e63d0424c7756d021ed5b0cb76a3",
      "path": "db/salescenter/migrations/202301031500_subscription-alter.up.sql",
      "mode": "100644",
      "type": "blob",
      "url": "https://api.github.com/repos/gravitational/cloud/git/trees/293bbd2c51c1e63d0424c7756d021ed5b0cb76a3"
    }],
	"truncated": false
	}`
	err := json.Unmarshal([]byte(treeJSON), &tree)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	cases := []struct {
		path   string
		expect []string
	}{
		{ // 0
			path: "",
			expect: []string{
				".drone.yml",
				"db/salescenter/migrations/202301031000_product-alter.up.sql",
				".github/ISSUE_TEMPLATE/tenant_upgrade.md",
				"db/salescenter/migrations/202301031500_subscription-alter.up.sql",
			},
		},
		{ // 1
			path: "db/salescenter/migrations",
			expect: []string{
				"db/salescenter/migrations/202301031000_product-alter.up.sql",
				"db/salescenter/migrations/202301031500_subscription-alter.up.sql",
			},
		},
	}
	for i, test := range cases {
		got := findTreeBlobEntries(tree, test.path)
		if len(got) != len(test.expect) {
			t.Fatalf("[%d] expected %d items got %d", i, len(test.expect), len(got))
		}
		for j := range got {
			if got[j] != test.expect[j] {
				t.Errorf("[%d:%d] expected '%s' got '%s'", i, j, test.expect[j], got[j])
			}
		}
	}
}
