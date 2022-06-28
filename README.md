# go-gitea-webhook

Simple webhook receiver implementation for Gitea/Gogs. Based on [go-gitlab-webhook](https://github.com/soupdiver/go-gitlab-webhook) and [go-gitea-webhook](github.com/mrexodia/go-gitea-webhook).

## Installation guide

First get and build `go-gitea-webhook`:

```bash
go install github.com/jum/go-gitea-webhook@latest
```

Then set up your configuration in `config.json`:

```json
{
  "logfile": "go-gitea-webhook.log",
  "address": "0.0.0.0",
  "port": 3344,
  "secret": "verysecret123",
  "repositories": [
    {
      "name": "user/repo",
      "commands": [
        "/home/user/update_repo.sh"
      ]
    }
  ]
}
```

Running `./go-gitea-webhook` should create `go-gitea-webhook.log` with content like this:

```
2018/02/15 06:05:29 Listening on 0.0.0.0:3344
```

The special logfile name of "-" is recognized to not redirect the lofile so it can be used from systemd-journald or launchd plists.

## Example use case

In my case I want to pull the changes to my server when someone pushes a new commit. I use [Caddy](https://caddyserver.com) and [supervisor](http://supervisord.org) to setup a simple service.

Contents of `update_repo.sh` (this will update a clone of `user/repo` to the latest `master` when executed, credentials are stored in the clone URL):

```bash
#!/bin/bash
old_pwd=$(pwd)
cd /home/user/repo
rm -rf .git/refs/heads/*
git fetch --prune origin
for branch in `git branch -a | grep remotes/origin | grep -v HEAD`; do
   git branch --track ${branch#remotes/origin/} $branch
done
git checkout -q origin/master
cd $old_pwd
```

The following special environment names are set while executing the hook script:

```bash
REPO_NAME="user/repo"
REPO_REF="refs/head/master"
REPO_AFTER="<commit_hash>"
```

Supervisor config (`/etc/supervisor/conf.d/webhook.conf`):

```
[program:webhook]
directory=/home/user/go/src/github.com/mrexodia/go-gitea-webhook
command=/home/user/go/src/github.com/mrexodia/go-gitea-webhook/go-gitea-webhook
autostart=true
autorestart=true
startsecs=10
user=user
environment=HOME="/home/user", USER="user"
minfds=8192
```

Contents of the `Caddyfile` (see [documentation](https://caddyserver.com/docs/hook.service) on how to setup a Caddy service):

```
webhook.example.com {
	proxy / http://127.0.0.1:3344 {
                transparent
        }
}
```

To set up the webhook, add a _Gitea_ webhook with the following configuration:

```
Payload URL: https://webhook.example.com
Content Type: application/json
Secret: verysecret123
When should this webhook be triggered? Just the push event
```

When the webhook is triggered (either by pushing or by using the *Test delivery* button) something along these lines should be appended to `go-gitea-webhook.log`:

```
2018/02/15 06:28:51 RemoteAddr: 127.0.0.1:53778
2018/02/15 06:28:51 received webhook on user/repo
2018/02/15 06:28:55 Executed: /home/user/update_repo.sh
2018/02/15 06:28:55 Output: Branch 'master' set up to track remote branch 'master' from 'origin'.
```
