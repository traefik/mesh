# Contributing

Traefik Mesh is an open source project, and your feedback and contributions are needed and always welcome.

[Issues] and [Pull Requests] are opened at https://github.com/traefik/mesh.

Non trivial changes should be discussed with the project maintainers by opening a [Feature Request] clearly explaining rationale, 
background and possible implementation ideas. Feel free to provide code in such discussions.

Once the proposal is approved, a Pull Request can be opened. If you want to provide early visibility to reviewers, create a [Draft Pull Request].

[Issues]: https://github.com/traefik/mesh/issues
[Pull Requests]: https://github.com/traefik/mesh/issues
[Feature Request]: https://github.com/traefik/mesh/issues/new?template=feature_request.md
[Draft Pull Request]: https://github.blog/2019-02-14-introducing-draft-pull-requests/

## Release Process

Traefik Mesh follows the [semver](https://semver.org/) scheme when releasing new versions.

Therefore, all PR's (except fixing a bug in a specific version) should be made against the `master` branch.
If you're attempting to fix a bug in an already released version, please use the correct branch of that release (e.g. `v1.1`).
All bug-fixes made to a specific branch will be backported to master, prior to releasing a new (major / minor) version. Patch releases will be made out of the version branch.

### Release steps

In order to release a new version of Traefik Mesh, the following steps have to be done:

* Create a Prepare release PR updating the chart version and app version to upcoming release
* Prepare a list of release notes for the #traefik-mesh
* Merge Prepare release PR
* Create and push a tag on the release commit
* Create a new release branch (v1.x) on upstream to allow versioned docs to be built
