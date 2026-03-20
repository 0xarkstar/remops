package security

import "testing"

func TestValidateHostName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid alphanumeric", "host1", false},
		{"valid with hyphen", "my-host", false},
		{"valid with underscore", "my_host", false},
		{"valid with dot", "host.local", false},
		{"valid mixed", "web-1_prod.local", false},
		{"empty", "", true},
		{"semicolon", "host;bad", true},
		{"pipe", "host|bad", true},
		{"ampersand", "host&bad", true},
		{"dollar", "host$bad", true},
		{"backtick", "host`bad", true},
		{"open paren", "host(bad", true},
		{"close paren", "host)bad", true},
		{"open brace", "host{bad", true},
		{"less than", "host<bad", true},
		{"greater than", "host>bad", true},
		{"backslash", "host\\bad", true},
		{"space", "host bad", true},
		{"newline", "host\nbad", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHostName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateHostName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateHostName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my-service_1", false},
		{"valid with dot", "svc.prod", false},
		{"empty", "", true},
		{"bang", "svc!bad", true},
		{"at", "svc@bad", true},
		{"space", "svc bad", true},
		{"slash", "svc/bad", true},
		{"colon", "svc:bad", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateServiceName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateServiceName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateServiceName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "myapp_container", false},
		{"valid with dot", "my.container", false},
		{"valid with hyphen", "my-container-1", false},
		{"empty", "", true},
		{"semicolon", "container;evil", true},
		{"dollar", "container$bad", true},
		{"space", "container bad", true},
		{"slash", "container/bad", true},
		{"backtick", "container`bad", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContainerName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateContainerName(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateContainerName(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}

func TestDetectShellInjection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"clean", "docker ps", false},
		{"clean with flags", "docker logs --tail 100", false},
		{"semicolon", "cmd; rm -rf /", true},
		{"pipe", "cmd | cat /etc/passwd", true},
		{"double ampersand", "cmd && evil", true},
		{"backtick", "cmd `whoami`", true},
		{"dollar", "cmd $HOME", true},
		{"open paren", "cmd (sub)", true},
		{"close paren", "cmd sub)", true},
		{"open brace", "cmd {x}", true},
		{"less than", "cmd < /etc/passwd", true},
		{"greater than", "cmd > /tmp/out", true},
		{"backslash", "cmd\\n", true},
		{"newline", "cmd\n evil", true},
		{"carriage return", "cmd\r evil", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := DetectShellInjection(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("DetectShellInjection(%q): expected error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("DetectShellInjection(%q): unexpected error: %v", tc.input, err)
			}
		})
	}
}
