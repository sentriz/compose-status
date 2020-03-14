### compose status

- keep an eye on your compose projects  
- list container status in a web interface
- groups containers based on compose projects
- groups compose projects on custom name
  - add a `xyz.senan.compose-status.group` label to any container in a project
- traefik aware. can generate clickable links based on host labels  

### docker example

```yaml
services:
  status:
    image: sentriz/compose-status
    environment:
    # see the `-h` for all args. they translate to env
    # variables with a `CS_` prefix
    - CS_PAGE_TITLE=my.domain status
    - CS_SCAN_INTERVAL=5
    - CS_HIST_WINDOW=1800
    expose:
    - 80
    volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
    - /proc:/host_proc:ro
    - ./data:/data
```

### screenshot

![](https://i.imgur.com/o8U3qlq.png)
