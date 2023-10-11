# Kinesis

Kinesis is an extensible model for developing event-driven microservices in golang. The software architecture is designed 
on the [C4](https://c4model.com) model, making it easy to separate various functionalities which serve a purpose, into 
separate <b>components</b>. A <b>container</b> comprises of those components which eventually make up a system.

A system could also be viewed as a collection of containers, each catering to an aspect of the overall system and therefore
a container could be considered as a component. In addition to modelling container <-> component heirachies emphasis is also provided on how components act on a stimuli (```events or messages```) and communicate with each other.

Each component gets to implement its own logic within each lifecycle stage, namely ```preinit, Init, PostInit, Start, Stop``` etc. The stage to which a component transitions next, is determined by a simple state machine (SM), generically implemented to oversee a component. Due to this generic nature, the SM is not involved with how each component would run within a stage or how it recovers from an error condition.

Depending on the stimuli received, a component has the flexibility to alter its own state at any lifecycle stages, but at the same time is also subject to change by the encompassing container. For e.g just when a component is processing an event stimuli which causes the state to "Restart", the parent container would have received a system interrupt to shutdown all the components under it. So at any point in time, a component state is being determined by 2 or more separate goroutines which are concurrently being executed in different contexts.

Add-on components such as an 'HTTP server' or 'Persistence' can be selectively included and activated by applying the appropriate Golang build tags during the build or 'go run' execution.
<br/>

### Component design
The ```SimpleComponent``` type implements all the methods within a Component interface. Additional component types could be composed by including SimpleComponent as an embedded type thereby acquiring all the methods of the embedded type, which could be overridden by the embedding component. Container types could be composed by embedding the ```Container``` type, which in turn embeds the SimpleComponent type.

<br/>

### Dynamic HTTP URI
The framework dynamically creates HTTP URIs' for all exported methods within a component, which resemble an HTTP handler function ```func (w http.ResponseWriter, r *http.Request)```. At the time of initializing a component, HTTP URIs' are derived from the component type and added to a HTTP handler map, maintained within the parent container, for purpose of forwarding HTTP requests. Corresponding HTTP handler entries are removed in the event a component is being stopped and teared down. 

To activate the HTTP server and expose components as HTTP endpoints, build or run with build tag ```-tags=http```
<br/>

### Persistence
TODO

<br/>

### A note on resiliency
The framework consists of a peculiar design consideration to always push a Golang defer function to be executed within a separate goroutine, in the event a state function finishes execution. In order to acheive this, a state method would always start with a defer function being included first. The defer function always includes a call to the State Machine(SM), to decide which stage it needs to transition next. This level of resilience guarantees that the call to SM would get pushed to the call stack no matter if a component panics or errors out. In addition, the defer function is executed within a separate goroutine in order to avoid cyclic call, which otherwise could lead to a stack overflow.

<br/>

### Building and Running
- ```available tags: http, simplefiledb```
- ```go build -tags=<comma separated build tags> cmd/kinesis.go```
- ```./kinesis```
- ```ctrl-c to quit```

<br/>