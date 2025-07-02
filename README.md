This is an example how to watch kubernetes statefulset for restart of containers.
I had the problem that, when a container is restarted for some reason such as OOM or failed liveness probe, it has not only to restart the container, but the whole pod. 
With this deployment, it is possible to achieve that. In case of a failed container it will restart both pods. That includes than also the init pods.
