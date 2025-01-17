package mqtt

import (
	"context"
	"os"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cloudeventscontext "github.com/cloudevents/sdk-go/v2/context"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

const testSourceConfig = `
brokerHost: test
topics:
  sourceEvents: sources/hub1/clusters/+/sourceevents
  agentEvents: sources/hub1/clusters/+/agentevents
`

func TestSourceContext(t *testing.T) {
	file, err := os.CreateTemp("", "mqtt-config-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	if err := os.WriteFile(file.Name(), []byte(testSourceConfig), 0644); err != nil {
		t.Fatal(err)
	}

	options, err := BuildMQTTOptionsFromFlags(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		event         cloudevents.Event
		expectedTopic string
		assertError   func(error)
	}{
		{
			name: "unsupported event",
			event: func() cloudevents.Event {
				evt := cloudevents.NewEvent()
				evt.SetType("unsupported")
				return evt
			}(),
			assertError: func(err error) {
				if err == nil {
					t.Errorf("expected error, but failed")
				}
			},
		},
		{
			name: "resync status",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				return evt
			}(),
			expectedTopic: "sources/hub1/clusters/cluster1/sourceevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
		{
			name: "unsupported send resource no cluster name",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				return evt
			}(),
			assertError: func(err error) {
				if err == nil {
					t.Errorf("expected error, but failed")
				}
			},
		},
		{
			name: "send spec",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("clustername", "cluster1")
				return evt
			}(),
			expectedTopic: "sources/hub1/clusters/cluster1/sourceevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sourceOptions := &mqttSourceOptions{
				MQTTOptions: *options,
				sourceID:    "hub1",
			}
			ctx, err := sourceOptions.WithContext(context.TODO(), c.event.Context)
			c.assertError(err)

			topic := func(ctx context.Context) string {
				if ctx == nil {
					return ""
				}

				return cloudeventscontext.TopicFrom(ctx)
			}(ctx)

			if topic != c.expectedTopic {
				t.Errorf("expected %s, but got %s", c.expectedTopic, topic)
			}
		})
	}
}
