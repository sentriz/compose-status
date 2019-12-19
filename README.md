### compose status

- keep an eye on your compose projects  
- list container status in a web interface, grouped by compose project  
- traefik aware. can generate clickable links based on host labels  
- no config if you don't want to   
- show containers that have been down for longer then a specified time (default 3 days) after that they're forgotten  

### docker example

```yaml
services:
  status:
    image: sentriz/compose-status
    environment:
    # see the `-h` for all args. they translate to env
    # variables with a `CS_` prefix
    - CS_PAGE_TITLE=my.domain status
    - CS_CLEAN_CUTOFF=259200
    - CS_SCAN_INTERVAL=5
    expose:
    - 80
    volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
    - /proc:/host_proc:ro
    - ./data:/data
```

### screenshot

![](https://i.imgur.com/o8U3qlq.png)
