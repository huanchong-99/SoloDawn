Feature: Outbound checks on an AI's post
  Scenario: A pasted private key header is refused without echoing the text
    When the host checks this AI post for secrets:
      """
      The file starts with -----BEGIN PRIVATE KEY-----
      """
    Then the outbound secret check refuses it with "secret: this looks like a secret; remove it and send again"

  Scenario: A pasted environment block with a sensitive name is refused
    When the host checks this AI post for secrets:
      """
      Here is the configuration:
      MODE=test
      SERVICE_TOKEN=placeholder
      PORT=8000
      """
    Then the outbound secret check refuses it with "secret: this looks like a secret; remove it and send again"

  Scenario: Mentioning a secret in an ordinary value is allowed
    When the host checks this AI post for secrets:
      """
      MODE=test
      MESSAGE=Set the SERVICE_TOKEN before starting
      PORT=8000
      """
    Then the outbound secret check allows it
