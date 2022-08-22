# Kinesis

Kinesis is an extensible model for developing event-driven microservices in golang. The software architecture is designed 
on the [C4](https://c4model.com) model, making it easy to separate various functionalities which serve a purpose, into 
separate <b>components</b>. A <b>container</b> comprises of those components which eventually make up a system.

A system could also be viewed as a collection of containers, each catering to an aspect of the overall system and therefore
a container could be considered as a component. In addition to modelling container <-> component heirachies emphasis is also provided on how components act on a stimuli (```events or messages```) and communicate with each other.

Each component gets to implement its own logic within each lifecycle stage, namely ```preinit, init, Start, Stop``` etc. The stage to which a component transitions next, is determined by a simple state machine (SM), generically implemented to oversee a component. Due to this generic nature, the SM is not involved with how each component would run within a stage or how it recovers from an error condition.

Depending on the stimuli received, a component has the flexibility to alter its own state at any lifecycle stages, but at the same time is also subject to change by the encompassing container. For e.g just when a component is processing an event stimuli which caused it to set state to "Restarting", the parent container would have received a system interrupt to shutdown all the components under it. So at any point in time, a component state is being determined by 2 or more separate goroutines which are concurrently being executed in different contexts.

<br/>

### A note about resiliency
The framework consists of a peculiar design consideration to always push a Golang defer function to be executed within a separate goroutine, in the event a state function finishes execution. In order to acheive this, a state method would always start with a defer function being included first. The defer function always includes a call to the State Machine(SM), to decide which stage it needs to transition next. This level of resilience guarantees that the call to SM would get pushed to the call stack no matter if a component panics or errors out. In addition, the defer function is executed within a separate goroutine in order to avoid cyclic call, which could result in a stack overflow.


<br/>

### Building and Running
- ```go build cmd/kinesis.go```
- ```./kinesis```
- ```ctrl-c to quit```

<br/>

### TODO
- [X] add dynamic http routes addition/deletion
- [X] design on demo app to showcase functionalities
- [X] refactor notifications with timeout
- [X] implement container Stop to stop components maintained within it
- [X] root container to block until interrupt signal from OS
- [X] check mechanism for returning copy of initialized component from container.
- [X] using component hash & etag to accept/reject component type messages to update.
- [X] add more UT to cover all mechanisms developed till now.
- [X] additional UT cases to add: 
    - [X] simulate app os signal interrupt handling.
    - [X] component name permissable character check.
    - [X] check for adding route to exported http handler func.
    - [X] check sequential order of component activation.