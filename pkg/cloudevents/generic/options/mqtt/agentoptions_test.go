package mqtt

import (
	"context"
	"os"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cloudeventscontext "github.com/cloudevents/sdk-go/v2/context"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

const testAgentConfig = `
brokerHost: test
topics:
  sourceEvents: sources/hub1/clusters/+/sourceevents
  agentEvents: sources/hub1/clusters/+/agentevents
  agentBroadcast: clusters/+/agentbroadcast
`

var mockEventDataType = types.CloudEventsDataType{
	Group:    "resources.test",
	Version:  "v1",
	Resource: "mockresources",
}

func TestAgentContext(t *testing.T) {
	file, err := os.CreateTemp("", "mqtt-config-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	if err := os.WriteFile(file.Name(), []byte(testAgentConfig), 0644); err != nil {
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
			name: "resync specs",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceSpec,
					Action:              types.ResyncRequestAction,
				}

				evt := cloudevents.NewEvent()
				evt.SetType(eventType.String())
				evt.SetExtension("originalsource", types.SourceAll)
				evt.SetExtension("clustername", "cluster1")
				return evt
			}(),
			expectedTopic: "clusters/cluster1/agentbroadcast",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
		{
			name: "send status no original source",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetSource("hub1")
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
			name: "send status",
			event: func() cloudevents.Event {
				eventType := types.CloudEventsType{
					CloudEventsDataType: mockEventDataType,
					SubResource:         types.SubResourceStatus,
					Action:              "test",
				}

				evt := cloudevents.NewEvent()
				evt.SetSource("agent")
				evt.SetType(eventType.String())
				evt.SetExtension("originalsource", "hub1")
				return evt
			}(),
			expectedTopic: "sources/hub1/clusters/cluster1/agentevents",
			assertError: func(err error) {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			agentOptions := &mqttAgentOptions{
				MQTTOptions: *options,
				clusterName: "cluster1",
			}
			ctx, err := agentOptions.WithContext(context.TODO(), c.event.Context)
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
