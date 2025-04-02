package plan

import (
	"context"
	"fmt"

	"github.com/gookit/color"

	"github.com/werf/nelm/internal/log"
)

func LogPlannedChanges(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	releaseChangesPlanned bool,
	createdChanges []*CreatedResourceChange,
	recreatedChanges []*RecreatedResourceChange,
	updatedChanges []*UpdatedResourceChange,
	appliedChanges []*AppliedResourceChange,
	deletedChanges []*DeletedResourceChange,
) {
	totalChangesLen := len(createdChanges) + len(recreatedChanges) + len(updatedChanges) + len(appliedChanges) + len(deletedChanges)

	if totalChangesLen == 0 {
		if releaseChangesPlanned {
			log.Default.Info(ctx, color.Style{color.Bold, color.Yellow}.Render(fmt.Sprintf("No changes planned, but will create release %q (namespace: %q)", releaseName, releaseNamespace)))
		} else {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("No changes planned for release %q (namespace: %q)", releaseName, releaseNamespace)))
		}

		return
	}

	log.Default.Info(ctx, "")

	for _, change := range createdChanges {
		log.Default.InfoBlock(ctx, createStyle("Create ")+resourceStyle(change.ResourceID.HumanID())+ending(change.CleanedUpOnSuccess, change.CleanedUpOnFailure)).Do(
			func() {
				log.Default.Info(ctx, "%s", change.Udiff)
			},
		)
	}

	for _, change := range recreatedChanges {
		log.Default.InfoBlock(ctx, recreateStyle("Recreate ")+resourceStyle(change.ResourceID.HumanID())+ending(change.CleanedUpOnSuccess, change.CleanedUpOnFailure)).Do(
			func() {
				log.Default.Info(ctx, "%s", change.Udiff)
			},
		)
	}

	for _, change := range updatedChanges {
		log.Default.InfoBlock(ctx, updateStyle("Update ")+resourceStyle(change.ResourceID.HumanID())+ending(change.CleanedUpOnSuccess, change.CleanedUpOnFailure)).Do(
			func() {
				log.Default.Info(ctx, "%s", change.Udiff)
			},
		)
	}

	for _, change := range appliedChanges {
		log.Default.InfoBlock(ctx, applyStyle("Blindly apply ")+resourceStyle(change.ResourceID.HumanID())+ending(change.CleanedUpOnSuccess, change.CleanedUpOnFailure)).Do(
			func() {
				log.Default.Info(ctx, "%s", change.Udiff)
			},
		)
	}

	for _, change := range deletedChanges {
		log.Default.InfoBlock(ctx, deleteStyle("Delete ")+resourceStyle(change.ResourceID.HumanID())).Do(
			func() {
				log.Default.Info(ctx, "%s", change.Udiff)
			},
		)
	}

	log.Default.Info(ctx, color.Bold.Render("Planned changes summary")+" for release %q (namespace: %q):", releaseName, releaseNamespace)
	if len(createdChanges) > 0 {
		log.Default.Info(ctx, "- "+createStyle("create:")+" %d resource(s)", len(createdChanges))
	}
	if len(recreatedChanges) > 0 {
		log.Default.Info(ctx, "- "+recreateStyle("recreate:")+" %d resource(s)", len(recreatedChanges))
	}
	if len(updatedChanges) > 0 {
		log.Default.Info(ctx, "- "+updateStyle("update:")+" %d resource(s)", len(updatedChanges))
	}
	if len(appliedChanges) > 0 {
		log.Default.Info(ctx, "- "+applyStyle("blindly apply:")+" %d resource(s)", len(appliedChanges))
	}
	if len(deletedChanges) > 0 {
		log.Default.Info(ctx, "- "+deleteStyle("delete:")+" %d resource(s)", len(deletedChanges))
	}
	log.Default.Info(ctx, "")
}

func createStyle(text string) string {
	return color.Style{color.Bold, color.Green}.Render(text)
}

func recreateStyle(text string) string {
	return color.Style{color.Bold, color.LightGreen}.Render(text)
}

func updateStyle(text string) string {
	return color.Style{color.Bold, color.Yellow}.Render(text)
}

func applyStyle(text string) string {
	return color.Style{color.Bold, color.LightYellow}.Render(text)
}

func deleteStyle(text string) string {
	return color.Style{color.Bold, color.Red}.Render(text)
}

func resourceStyle(text string) string {
	return color.Style{color.Bold}.Render(text)
}

func ending(cleanupOnSuccess, cleanupOnFailure bool) string {
	if cleanupOnSuccess && cleanupOnFailure {
		return " and " + deleteStyle("delete") + " it"
	} else if cleanupOnSuccess {
		return " and " + deleteStyle("delete") + " it on success"
	} else if cleanupOnFailure {
		return " and " + deleteStyle("delete") + " it on failure"
	}

	return ""
}
