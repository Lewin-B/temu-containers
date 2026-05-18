## Summary

This is a runtime tool for creating and managing containers. It'll be used to power the apis in the temu-containerd package

### Commands

- Create
  - Initializes a container by creating the necessary directories and namespace, mounting the filesystem into it's namspace, and creating the cgroup
- Start
  - Starts a container by signaling to it's init process to proceed
- Run
  - Runs a container by running create and start consecutively
- Pause
  - Pauses all container procs by running freeze on it's cgroup service
- Resume
  - Resumes all container procs by running thaw on it's cgroup service
- Kill
  - Kills a container by running stop on it's cgroup service
- Delete
  - Cleans up a container by removing it from memory
