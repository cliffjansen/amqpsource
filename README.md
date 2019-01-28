# amqpsource
AMQP cloudevent source for knative eventing

## Work in Progress

Currently the source can only handle AMQP messages in a very particular format
due to limitations in the github.com/knative/pkg/cloudevents package:

The message must have content_type property "application/json" and a single Data
section that, when converted to a string, is valid input to
[json.Marshal()](https://golang.org/pkg/encoding/json/#Marshal).

In future, the AmqpSource will use the [lightning
library](https://github.com/alanconway/lightning) to handle arbitrary
AMQP messages, including messages [that already
encode](https://github.com/cloudevents/spec/blob/master/amqp-transport-binding.md)
a CloudEvents event.

## Install

1. Copy the subtree into knative/eventing-sources.

2. Build the source's receive adapter image:

   ```shell
   docker build . --tag amqp-adapter:xyz
   ```

3. Edit the value for AMQP_RA_IMAGE in config/default-amqp.yaml to point to this image.

4. Install the eventing-sources including the AmqpSource:

   ```shell
   ko apply -f config/default-amqp.yaml`
   ```
