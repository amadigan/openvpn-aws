---
layout: page
title: Configuration
permalink: /configuration/
---
# Configuration
Here is a sample configuration file:

vpn.conf:
```
global
  net 169.254.121.0/24
  route53 Z4C9QKLRTRVI8Q vpn.example.com
  watch 5m
  dns off

group rds-access
  dns on
  subnet-0c02187459d3a72b2

group ssh

group customer-service
  subnet-0c0897424d3a75b4

user joe
```

## Structure
The configuration file is divided into sections, sections always start with a line that does not start with whitespace
(the global, group, and user sections above). Each section can contain network rules like `subnet-`, `pcx-`, `nat`, `dns`.
The `global` section additionally contains server configuration that is the same for all groups. Rules within a section always
start with whitespace. Blank lines are ignored, as are anycharacters after a # symbol.
