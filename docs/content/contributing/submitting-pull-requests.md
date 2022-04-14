---
title: Submitting Pull Requests for Traefik Mesh | Traefik Docs
description: This documentation article describes the process of submitting a pull request for Traefik Mesh.
---

# Submitting Pull Requests

So you've decided to improve Traefik Mesh? Thank You! Now the last step is to submit your Pull Request in a way that makes sure 
it gets the attention it deserves.

Let's go through the classic pitfalls to make sure everything is right. 

## Title

The title must be short and descriptive. (~60 characters)

## Description

Follow the [pull request template](https://github.com/traefik/mesh/blob/master/.github/PULL_REQUEST_TEMPLATE.md) 
as much as possible.

Explain the conditions which led you to write this PR: give us context. The context should lead to something, an idea or 
a problem that you’re facing.

Remain clear and concise.

Take time to polish the format of your message so we'll enjoy reading it and working on it. Help the readers focus on 
what matters, and help them understand the structure of your message (see the [Github Markdown Syntax](https://help.github.com/articles/github-flavored-markdown)).

## Content

- Make it small.
- Each PR should be linked to an issue.
- One feature per PR.
- PRs should be standalone (they should not depend on an upcoming PR).
- Write useful descriptions and titles.
- Avoid re-formatting code that is not on the path of your PR.
- Commits should be split properly (in order to guide reviewers through the code).
- Make sure the [code builds](building-testing.md).
- Make sure [all tests pass](building-testing.md).
- Add tests.
- Address review comments in terms of additional commits (don't amend/squash existing ones unless the PR is trivial).
