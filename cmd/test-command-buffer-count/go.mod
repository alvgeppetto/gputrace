module test

go 1.25.3

replace github.com/tmc/mlx-go/experiments/gputrace => ../..

require github.com/tmc/mlx-go/experiments/gputrace v0.0.0-00010101000000-000000000000

require (
	github.com/google/pprof v0.0.0-20251007162407-5df77e3f7d1d // indirect
	howett.net/plist v1.0.1 // indirect
)
