package todoui

// version is the default shown in-app; release builds override it with
// -ldflags "-X github.com/grassbl8d/todo-ui/internal/todoui.version=vX.Y.Z".
// Between releases this carries a "-dev" snapshot suffix for the upcoming
// version; scripts/release.sh strips it to cut the clean release, then bumps it
// to the next "-dev" — don't edit it by hand.
var version = "v0.2.3"
