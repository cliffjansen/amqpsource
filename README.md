# amqpsource
AMQP cloudevent source for knative eventing

## Work in Progress

Currently the source can only handle AMQP messages in a very particular format
due to limitations in the github.com/knative/pkg/cloudevents package:

The message must have content_type property "application/json" and a single Data
section that when converted to a string is valid input to
[json.Marshal()](https://golang.org/pkg/encoding/json/#Marshal).
