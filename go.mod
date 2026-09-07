module github.com/ehsaniara/joblet-flow

go 1.24.0

require (
	github.com/ehsaniara/joblet-proto/v2 v2.5.10
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.34.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	golang.org/x/net v0.23.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
)

// The proto contract is developed alongside the engine; resolve it from the
// sibling checkout rather than the module proxy.
replace github.com/ehsaniara/joblet-proto/v2 => ../joblet-proto
