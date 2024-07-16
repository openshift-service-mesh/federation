### About the project

`Federation` is a controller that utilizes MCP protocol to configure mesh-federation in Istio.

### Development
1. Build:
```shell
make
```
2. Run locally:
```shell
./out/federation-controller \
  --meshPeers '{"spec":{"remote":{"addresses": ["lb-1234567890.us-east-1.elb.amazonaws.com","192.168.10.56"],"ports":{"dataPlane":15443,"discoery":15020}}}}'\
  --exportedServiceSet '{"type":"LabelSelector","labelSelectors":[{"matchLabels":{"export-service":"true"}}]}'\
  --importedServiceSet '{"type":"LabelSelector","labelSelectors":[{"matchLabels":{"export-service":"true"}}]}'
```
