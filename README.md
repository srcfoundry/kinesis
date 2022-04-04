# Kinesis

Kinesis is an extensible framework reference for building event-driven microservices. 
<p>Common functionalities are modeled as <b>components</b>, which communicate with each other by means of passing messages or notifications.</p>


<br/>

### TODO
- [X] implement container Stop to stop components maintained within it
- [X] root container to block until interrupt signal from OS
- [X] check mechanism for returning copy of initialized component from container.
- [X] using component hash & etag to accept/reject component type messages to update.
- [ ] add more UT to cover all mechanisms developed till now.