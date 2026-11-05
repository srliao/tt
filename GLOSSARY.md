# tt

A single-user task tracker where userscripts create tasks on a schedule, so recurring chores appear on their own.

## Tasks

**Task**:
One unit of work with a title, notes, optional due date and a state.
_Avoid_: Todo, item, ticket

**State**:
Where a task is in its life: `not_done`, `done` or `cancelled`.
_Avoid_: Status (that word belongs to script runs)

**Open task**:
A task in the `not_done` state.
_Avoid_: Active, pending, incomplete

**Priority**:
A task's position in the main task list, set by hand.
_Avoid_: Rank, importance

**Stage**:
A hand-picked, separately ordered set of tasks being worked through now. Finished tasks stay visible in it.
_Avoid_: Queue, today list, focus list

**Tag**:
A named label attached to tasks for filtering, with a fixed color.
_Avoid_: Label, category

## Dependencies

**Dependency**:
A link saying one task cannot be completed until another is resolved. Only one kind of link exists between tasks.
_Avoid_: Subtask, parent, child, relation

**Blocker**:
The task that a dependency waits on.
_Avoid_: Prerequisite, upstream

**Dependent**:
The task that waits on a blocker.
_Avoid_: Downstream, child

**Blocked**:
A task with at least one blocker that is still open or cancelled. Never stored, always worked out from its blockers.
_Avoid_: Waiting, on hold

**Ready**:
An open task that is not blocked.
_Avoid_: Actionable, available, next

**Goal**:
A task that other tasks block, used as the end point of a plan. Not a separate kind of task.
_Avoid_: Epic, project, milestone

## Scripts

**Script**:
User-written JavaScript that runs on a schedule in a sandbox and can create tasks.
_Avoid_: Automation, job, rule

**Run**:
One execution of a script, with a status, logs and the tasks it created.
_Avoid_: Execution, job

**Spawn**:
A task created by a script run.
_Avoid_: Generated task, child task

**Drawing**:
A saved Excalidraw canvas, kept separate from tasks.
_Avoid_: Diagram, sketch, board
