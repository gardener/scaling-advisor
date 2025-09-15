package inmclient

import (
	appsv1 "k8s.io/api/apps/v1"
	"testing"
)

func Test_typeName(t *testing.T) {
	t.Logf("replicaset type name %q", typeName[*appsv1.ReplicaSet]())
}
