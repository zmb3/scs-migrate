# SCS Migrate

[![Build Status](https://travis-ci.com/zmb3/scs-migrate.svg?token=rV9CBTWmzXChoM9GCTab&branch=master)](https://travis-ci.com/zmb3/scs-migrate)

A tool to help you migrate the Spring Cloud Services tile
from version 2.x to version 3.x.

In its current state, this tool will help you audit your environment
and highlight any potential issues as you plan to update.

It does not currently perform any of the steps for
[migrating service instances](https://docs.pivotal.io/spring-cloud-services/3-1/common/config-server/managing-service-instances.html#migrating-2-0-or-1-5-service-instances).

## Features

- Identify all SCS service instances and apps that are bound to them
- Flag any Circuit Breaker Dashboards, which are
  [not available in SCS 3](https://tanzu.vmware.com/content/practitioners/replacing-the-spring-cloud-services-circuit-breaker-dashboard).
- Flag Eureka Service Registry instances, which are not available in SCS 3.0
  but are available in SCS 3.1+.
- Check for incompatible configuration parameters in config server instances,
  and warn appropriately.
