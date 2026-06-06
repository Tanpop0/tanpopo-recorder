package workerproc

import "testing"

func TestIsWorkerCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "explicit worker flag", args: []string{"--worker", "--job", "job.json"}, want: true},
		{name: "job flag is enough", args: []string{"--job", "job.json"}, want: true},
		{name: "worker subcommand", args: []string{"worker", "--job", "job.json"}, want: true},
		{name: "regular gui args", args: []string{"--foo"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWorkerCLI(tt.args); got != tt.want {
				t.Fatalf("IsWorkerCLI(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
