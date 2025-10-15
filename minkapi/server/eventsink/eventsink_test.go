package eventsink

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var log = klog.NewKlogr()

func TestCreateListReset(t *testing.T) {
	sink := New(log)
	var err error
	var e1, e2 *eventsv1.Event
	e1 = &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e1",
			Namespace: metav1.NamespaceDefault,
		},
		EventTime:           metav1.NewMicroTime(time.Now()),
		ReportingController: "xianxia",
		ReportingInstance:   "martial-arts",
		Action:              "KICK",
		Reason:              "Taekwondo Kick",
		Note:                "Taekwondo Session",
		Type:                "Warning",
	}
	e2 = e1.DeepCopy()
	e2.Name = "e2"
	e2.Action = "PUNCH"
	e2.EventTime = metav1.NewMicroTime(e1.EventTime.Add(time.Second * 10))
	e2.Reason = "Karate Fist"
	e2.Note = "Karate Session"

	t.Run("create", func(t *testing.T) {
		e1, err = sink.Create(t.Context(), e1)
		if err != nil {
			t.Errorf("cannot create e1 event: %v", err)
			return
		}
		e2, err = sink.Create(t.Context(), e2)
		if err != nil {
			t.Errorf("cannot create e2 event: %v", err)
			return
		}
	})
	t.Run("list", func(t *testing.T) {
		got := sink.List()
		want := []eventsv1.Event{*e1, *e2}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("eventList mismatch (-want +got):\n%s", diff)
		}
	})
	t.Run("reset", func(t *testing.T) {
		sink.Reset()
		if sink.List() != nil {
			t.Errorf("eventList should be empty")
		}
	})
}
