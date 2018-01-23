mackerel-plugin-aws-s3-requests
=================================

AWS S3 requests metrics plugin for mackerel.io agent.

## Requirement

You need a metrics configuration FilterID for the target S3 bucket to get CloudWatch metrics for the bucket. If you haven't created any configurations yet, you can create by AWS CLI or AWS API. (ref: https://docs.aws.amazon.com/AmazonS3/latest/dev/metrics-configurations.html)

## Synopsis

```shell
mackerel-plugin-aws-s3-requests -bucket-name=<bucket-name> -filter-id=<filter-id> -region=<aws-region> -access-key-id=<id> -secret-access-key=<key> [-tempfile=<tempfile>] [-metric-key-prefix=<prefix>] [-metric-label-prefix=<prefix>]
```
* `filter-id` is the id for the metrics configuration, described in "Requirement" section.
* you can set some parameters by environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`.
  * If both of those environment variables and command line parameters are passed, command line parameters are used.
* You may omit `region` parameter if you're running this plugin on an EC2 instance running in same region with the target Lambda function

## Example of mackerel-agent.conf

```
[plugin.metrics.aws-s3-requests]
command = "/path/to/mackerel-plugin-aws-s3-requests -bucket-name=MyBucket -filter-id=SomeFilterId -region=ap-northeast-1"
```

## License

Copyright 2018 astj

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
