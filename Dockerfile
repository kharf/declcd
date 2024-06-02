# Copyright 2024 kharf
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
FROM alpine:3.14 as builder
RUN apk add --no-cache git
RUN ls -la /usr/bin/

FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=builder --chmod=0700 --chown=65532:65532 /usr/bin/git /usr/bin/git 
COPY dist/controller_linux_amd64_v1 .

USER 65532:65532
ENTRYPOINT ["/controller"]
