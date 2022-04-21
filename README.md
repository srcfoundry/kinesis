# Kinesis

Kinesis is an extensible framework reference for building event-driven microservices. 
<p>Common functionalities are modeled as <b>components</b>, which communicate with each other by means of passing messages or notifications.</p>

<br/>

### Building and Running
- ```go install golang.org/x/tools/cmd/stringer@latest```
- ```go generate ./...```
- ```go build cmd/kinesis.go```
- ```./kinesis```

<br/>

### TODO
- [X] add dynamic http routes addition/deletion
- [] design on demo app to showcase functionalities
- [] add persistence capability for stateful app
- [] defer registering for OS signal notifications until root container initialization
- [X] implement container Stop to stop components maintained within it
- [X] root container to block until interrupt signal from OS
- [X] check mechanism for returning copy of initialized component from container.
- [X] using component hash & etag to accept/reject component type messages to update.
- [X] add more UT to cover all mechanisms developed till now.