# Contract: what a browser shows about a home of another (step 2)

In step 1 a browser shows what the home pushes, because the home is one of
the viewer's own devices. From step 2 a home may belong to someone else. By
the owner's rules of 2026-11-21 and 2026-11-22 a browser still shows what
the home pushes, as in step 1, and checks no envelope of a row: members of a
group trust each other, and security between people is outside the product.
This file settles design point D-6 of `steps/parallel-plan-step-2-3.md` and
what the interface of step 2 must say. It adds nothing to the wire. How it
looks is decided by the interface units.

## 1. The secretary warning (D-6)

There is no mechanism: R-51 stays without an auto-accept switch. The
interface warns, without blocking any action and with nothing to switch
on or off, and the documentation advises (section 3).

**Condition**, read from rows: the group row names a secretary, that
member row has `account: "personal"`, and the group has a person who is
not the secretary's owner (a `human` member whose `user` differs from the
secretary's `user`). The account type is what the owner declared (R-50);
the warning follows the declaration.

| Where | Shown to | When |
| --- | --- | --- |
| group info, beside the secretary | every person of the group | while the condition holds |
| choosing the secretary (`group set`, or adding an AI as secretary) | the person choosing | the choice would make the condition true |
| the invitation dialog | the person inviting | the secretary is on a personal subscription and the group has no other person yet |

**What it says**: every request to this secretary from someone other than
its owner waits until the owner confirms it, each time (R-51); with a team
plan or an API key the owner can list who may direct it (R-53).

## 2. Whose computer keeps the group

The group info shows, for every group, the name of the device this browser
is connected to as the home and the person whose device it is. For a home
of another it also says, once: that computer stores and orders the group and
must stay on (R-134).

## 3. Notes for the user documentation

| Topic | What it says |
| --- | --- |
| accounts in a team | A secretary that several people talk to should run on a team plan or an API key, where the owner lists who may direct it. On a personal subscription every request from someone else waits for the owner (R-51), and that cannot be switched off. The same holds for any AI that colleagues direct often |
| a group on someone else's computer | that computer stores the group, says who is a member and under which name, and decides which requests reach your AIs; join only groups of people you trust (deployment guide) |
