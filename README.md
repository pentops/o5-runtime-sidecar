O5 Runtime Sidecar
==================

This runs alongside an o5 application to adapt everything to the infrastructure.

The interface between the sidecar and the application is *mainly* gRPC

Currently it's just a JSON proxy.

Plan:

- Messaging via any number of underlying protocols
- Outbox pattern, postgres and potentially others
- CORS, Auth, Metrics, Logging
- Postgres auth proxy

The sidecar trusts the application's reflection proto, this is probably not
great but have to start somewhere. Comparing the reflection proto to some
central registry could be interesting but not in the initial scope.


Environment
-----------

Baseline Required:

`APP_NAME string`
`ENVIRONMENT_NAME string`

EventBridge

`EVENTBRIDGE_ARN string` - The ARN of the EventBridge bus to use

