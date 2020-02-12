# Building

Maesh can be built from source by running `make`, which will build a docker image of Maesh.
A binary will also be built as `./dist/maesh`, which can be run via a shell for testing.

## Integration testing options

For development purposes, you can specify which tests to run by using (only works the `test-integration` target):

```bash
# Run every tests in the MyTest suite
TESTFLAGS="-check.f MyTestSuite" make test-integration

# Run the test "MyTest" in the MyTest suite
TESTFLAGS="-check.f MyTestSuite.MyTest" make test-integration

# Run every tests starting with "My", in the MyTest suite
TESTFLAGS="-check.f MyTestSuite.My" make test-integration

# Run every tests ending with "Test", in the MyTest suite
TESTFLAGS="-check.f MyTestSuite.*Test" make test-integration
```
This will allow specific suites to be run.

More: https://labix.org/gocheck
