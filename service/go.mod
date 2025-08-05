module github.com/gardener/scaling-advisor/service

go 1.24.5

require (
	github.com/gardener/scaling-advisor/api v0.0.0
	github.com/gardener/scaling-advisor/common v0.0.0
)

replace (
	github.com/gardener/scaling-advisor/api => ../api
	github.com/gardener/scaling-advisor/common => ../common
)