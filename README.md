# gh actions-status

_being an extension to view the overall health of an organization's use of actions_

<img width="865" alt="screenshot of extension" src="https://user-images.githubusercontent.com/98482/156441347-a593d6a1-55ca-4911-a586-f88f18069ab6.png">

## Usage

By default, this command shows actions health for the last 30 days.

```bash
# See the actions health for an organization
gh actions-status cli

# See health for a different time period in either hours or days
gh actions-status -l 12h
gh actions-status -l 7d

# See health for an arbitrary list of repositories within an org
gh actions-status cli -r "cli,go-gh"

# See workflow runs with 'failure' status for an arbitrary list of repositories within an org
gh actions-status cli -r "cli,go-gh" -s "failure" 

# See the actions health for all the repositories of a user
gh actions-status rsese
```

## Installation

```bash
gh extension install rsese/gh-actions-status
```

## Authors

Robert Sese <rsese@github.com>, vilmibm <vilmibm@github.com>
