@web
Feature: Accessibility across the main-line pages and design widths
  Every main-line page exposes landmarks, headings, accessible names, language marks and live
  announcements, keeps keyboard focus on a meaningful part when dialogs, panels, cards or thread
  controls open, close or resolve, meets WCAG 2.2 AA contrast in light and dark themes, stills
  scripted motion under reduced motion, and adapts cleanly across the design's phone, tablet and
  desktop widths.

  Background:
    Given a browser set to English

  Scenario: Onboarding and settings expose landmarks, live announcements, names and language marks
    Given a fresh pairing browser on a 390 px phone
    When the person opens the client at "#/onboarding"
    Then the live status region stands outside the hidden top bar and announces theme, language and copy changes
    And the pairing prompt region is focusable and named "Prompt for your AI", and the pairing steps keep their list role inside a polite status region
    And every control on onboarding has an accessible name, and renewing an expired code keeps focus on the prompt action button
    When the person opens "#/settings" on a 1440 px desktop
    Then focus moves to the main landmark named "Settings", the skip button returns focus there, and each language choice is marked in its own language

  Scenario: Group list, waiting digest, create-group dialog, chat and cards name every control and link descriptions
    Given two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary
    When the person opens "#/groups" on a 1280 px desktop
    Then the group list and waiting digest expose named landmarks, and each digest action starts with "Open" and names its card and group
    When the person opens "New project group", checks its field names and descriptions, and presses Escape
    Then every dialog field is named by its label without concatenating its hint, and Escape closes the dialog with focus back on "New project group"
    When the person opens the chat of "Website" on a 1280 px desktop
    Then the chat, main line, waiting bar, composer and every card expose localized accessible names and linked status descriptions in English and Chinese

  Scenario: Acting on cards, threads, dialogs and computer accounts never drops focus to the document body
    Given two groups "Website" and "Docs" with waiting brief, approval and decision cards, a locked approval, a task and a summary
    And the chat of "Website" is open with 60 earlier progress messages on task 12
    When the person opens "Change it" on the brief card, presses Escape, reopens, cancels, and then confirms the brief
    Then focus moves to the note field, returns to "Change it" on Escape and Cancel while marking Escape handled, and lands on the folded brief summary on confirm
    When the person approves the approval card and chooses an option on the decision card from the keyboard
    Then focus stays on each card instead of falling to the document body, and returns to the action button when the home refuses an action
    When the person scrolls back, activates "Back to the latest", expands task 12's thread and loads the remaining earlier messages
    Then "Back to the latest" returns focus to the main line and finishing thread paging moves focus to the task's thread summary
    When the person opens "#/nodes", declares an AI account type from the keyboard, and opens, copies and closes "Add a browser" with Escape
    Then choosing the account keeps focus on the computer card, copying the pairing link announces "Copied", and Escape returns focus to "Add a browser"

  Scenario: Phone, tablet and desktop widths adapt whole-screen mode, connection indicator placement and side panel reachability
    Given a group "Website" with a side panel area and an active home connection
    When the person opens the chat of "Website" at 390, 820, 1024 and 1440 px widths
    Then on a 390 px phone the shell uses phone whole-screen mode and places the named connection indicator in the chat head
    And at 390 and 820 px the closed side panel is inert until opened, traps behind parts with inert when open, and restores focus on Escape
    And at 1024 and 1440 px the side panel stands beside the main line without inert on either column

  Scenario: Design tokens meet WCAG AA contrast in both themes and reduced motion stills scripted scrolling and transitions
    When the colour tokens of "src/styles/tokens.css" are evaluated in light and dark themes
    Then every text and badge colour pair meets at least 4.5 to 1 contrast and every control border and focus ring meets at least 3 to 1 contrast
    When the system asks for reduced motion
    Then jumping to a message, returning to the latest message and switching preferences run without smooth scrolling or view transitions, and the stylesheet keeps a single reduced-motion rule
