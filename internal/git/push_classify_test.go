package git

import "testing"

func TestClassifyPushStderr(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   PushFailureKind
	}{
		{"github-gh006", "remote: error: GH006: Protected branch update failed for refs/heads/teammemory.\n ! [remote rejected] teammemory -> teammemory (protected branch hook declined)", KindProtectedBranch},
		{"gitlab-protected", "remote: GitLab: You are not allowed to push code to protected branches on this project.\n ! [remote rejected] teammemory -> teammemory (pre-receive hook declined)", KindProtectedBranch},
		{"bitbucket-protected", " ! [remote rejected] teammemory -> teammemory (protected branch hook declined)", KindProtectedBranch},
		{"generic-pre-receive", " ! [remote rejected] teammemory -> teammemory (pre-receive hook declined)", KindProtectedBranch},
		{"auth-https", "remote: HTTP Basic: Access denied\nfatal: Authentication failed for 'https://example.com/org/repo.git/'", KindAuth},
		{"auth-ssh", "git@example.com: Permission denied (publickey).\nfatal: Could not read from remote repository.", KindAuth},
		{"auth-403", "remote: Permission to org/repo.git denied to alice.\nfatal: unable to access 'https://example.com/org/repo.git/': The requested URL returned error: 403", KindAuth},
		{"auth-username-prompt", "fatal: could not read Username for 'https://example.com': terminal prompts disabled", KindAuth},
		{"network-dns", "fatal: unable to access 'https://example.com/org/repo.git/': Could not resolve host: example.com", KindNetwork},
		{"network-refused", "fatal: unable to access 'https://example.com/org/repo.git/': Connection refused", KindNetwork},
		{"network-unreachable", "fatal: unable to access 'https://example.com/': Network is unreachable", KindNetwork},
		{"network-timeout", "fatal: unable to access 'https://example.com/': Operation timed out after 5000 milliseconds", KindNetwork},
		{"empty", "", KindUnknown},
		{"benign-unknown", "remote: blubber blubber\nerror: failed to push some refs", KindUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyPushStderr(tc.stderr)
			if got != tc.want {
				t.Fatalf("Classify(%q) = %q, want %q", tc.stderr, got, tc.want)
			}
		})
	}
}
