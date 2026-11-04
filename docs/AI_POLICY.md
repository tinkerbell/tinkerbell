# AI Policy

AI tools are welcome in Tinkerbell contributions. This policy helps you use them in a way that keeps contributions high quality and reviews productive for everyone.

## Rules

1. **You're accountable.** You must understand, test, and be able to explain every change you submit. "The AI did it" is not an answer. A PR whose author can't explain it will be closed.
2. **Disclose AI use in the PR description.** Name the tool and model (for example, "GitHub Copilot, Claude Sonnet 5"). In auto mode, check the tool's UI or ask the agent which model it used. Saying what you used it for is optional but helps reviewers.
3. **No AI trailers.** Don't list an AI tool in `Co-authored-by`, `Assisted-by`, `Signed-off-by`, or any similar commit trailer. Only you can sign the [DCO](https://github.com/tinkerbell/org/blob/main/DCO.md).
4. **Same quality bar.** AI-assisted code needs the same tests, docs, and design fit as any other code.
5. **Respect licenses.** Follow the [Linux Foundation Generative AI Policy](https://www.linuxfoundation.org/legal/generative-ai): the tool's terms must not restrict use of its output under this project's license, and any third-party content in the output must be permitted and attributed.
6. **Keep PRs small.** This applies to all PRs, AI-assisted or not. Each PR should be one focused change that a reviewer can understand on its own. Discuss large changes first in an issue or design doc, then split them into a series of small PRs.

PR descriptions, issues, and review replies may be AI-assisted if you disclose it and have checked the content is accurate.

## Take extra care with

- API and CRD changes (`api/`, `crd/`).
- Security-sensitive code: TLS, authentication, secrets, and `secondstar`.
- Network boot protocols in `smee` (DHCP, TFTP, iPXE).

## Security reports

Report vulnerabilities as described in [SECURITY.md](SECURITY.md). A report from an AI tool must include a reproduction you've run against a supported release. Unverified reports will be closed.

## Reviewers and maintainers

- AI review tools may be used, but a human must approve every merge.
- Resolve or dismiss incorrect or low-value comments from AI review tools on your PRs.

## Enforcement

Maintainers may close low-effort or undisclosed AI-generated issues and PRs without discussion. Repeated violations may lead to being blocked from the project.

## References

- [Linux Foundation Generative AI Policy](https://www.linuxfoundation.org/legal/generative-ai)
- [Kubernetes AI Guidance](https://github.com/kubernetes/community/blob/main/contributors/guide/pull-requests.md#ai-guidance)
- [OpenTelemetry Generative AI Policy](https://github.com/open-telemetry/community/blob/main/policies/genai.md)
- [Cluster API Provider AWS AI Usage Policy](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/AI_POLICY.md)
