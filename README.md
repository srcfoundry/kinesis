# Kinesis

Kinesis is an extensible model for developing event-driven microservices in golang. The software architecture is designed 
on the [C4](https://c4model.com) model, making it easy to separate various functionalities which serve a purpose, into 
separate <b>components</b>. A <b>container</b> comprises of those components which eventually make up a system.

A system could also be viewed as a collection of containers, each catering to an aspect of the overall system and therefore
a container could be considered as a component.

In addition to modelling container <-> component heirachies emphasis is also provided on how components could communicate with each other by means of passing messages or notifications.

<br/>

### Building and Running
- ```go build cmd/kinesis.go```
- ```./kinesis```
- ```ctrl-c to quit```

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