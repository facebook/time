# simpleclient
Basic PTPv2.1 two-step unicast client implementation.

## How to re-generate mocks

```console
mockgen -source=client.go -destination conn_mock_test.go -package simpleclient
```

and then update the license header
