# publish-directory

The `publish-directory` GitHub Action helps with publishing a specific directory as a branch. For example when publishing a terraform mono repository as isolated modules through branches which can be tagged. Another use case could be github pages.

`example`
```
- name: publish-directory
  uses: kontrolplane/publish-directory@v0.0.1
  with:
    branch: release/<name>
    folder: <directory-to-release>
    commit_username: "github-actions[bot]"
    commit_email: "github-actions[bot]@users.noreply.github.com"
    commit_message: "chore: update branch from directory"
```
