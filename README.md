# SCO: Simple container orchestrator

SCO is a simple container orchestrator, reverse proxy and load balancer.
It also allows forwarding requests/connections to servers in a private network.

It works by installing the sco-server on a publicly accessible server and sco-agent(s) on worker nodes.
The sco-server is responsible for planning container placement and load balancing.
The sco-agent(s) are responsible for running the containers.

In the sco-server you can define clusters and services.
A cluster is just a collection of services.
Services have the following properties:
- Service type (`tcp` or `http`)
- Name
- Container image
- Exposed ports
- Max CPU usage
- Max RAM usage
- Replicas
- Load balancing strategy (`random` or `round-robin`)
