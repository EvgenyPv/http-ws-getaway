# HTTP-WS-getaway
The gateway publishes "/api/device-ws" endpoint for websocket devices.

All connected devices required to send their device_id through GET parameter.

After opening connection devices will receive messages from "/api/send-message" endpoint.

Messages should be sent to "/api/send-message" via HTTP POST request and structured as follows:
```shell
{"id":"ffffffff-0000-1111-2222-334455667788",
"device_id":"00000000-0000-1111-2222-334455667788",
"kind":1,
"message":"Text message"}
```

# Testing strategy
To connect test device to websocket you can open "localhost:8080/", specify device_id and press "Open".
One browser tab allows to open one websocket connection.
To send message to "/api/send-message" endpoint you can use curl/postman or any other preffered API testing tool.

# Performance considerations
Currently we expect websocket send status to be recevied as HTTP POST reply.
This brings as to synchronous communications, which considerably limits application performance under high load.
If message send status is not expected by the sender, the following performance  improvements could be made:
1. Perform all the writes to websocket via single goroutine.
2. Communicate to abovementioned goroutine via buffered channel.
This would allow message sender to not depend on websocket write speed/status
In this case it probably makes sense to rewrite other goroutine synchronization using channels instead of mutex
