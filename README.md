The EGW operator manages Envoy pods based on our custom resources. The
data model that the operator watches is defined in
https://gitlab.com/acnodal/egw-resource-model. The operator code is
scaffolded by https://operatorframework.io/.

**NOTE**: Make sure that you've followed the egw-web-service
[Developer Setup
instructions](https://gitlab.com/acnodal/egw-web-service/-/tree/egw-resource-model#developer-setup)!
This project uses private golang modules so it won't build unless you
set up your system properly.

Run "make" to get a list of goals.
