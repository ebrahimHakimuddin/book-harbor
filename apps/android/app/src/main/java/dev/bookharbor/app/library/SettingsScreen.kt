package dev.bookharbor.app.library

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.ControlShape
import dev.bookharbor.app.ui.theme.cautionColor
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The app's settings: who you are and where you're connected, the device's own preferences,
 * and the app itself. Each row does one thing; editing opens a focused dialog.
 */
@Composable
internal fun SettingsTab(controller: AppController, catalog: LibraryUiState.Catalog) {
    val context = LocalContext.current
    var editName by rememberSaveable { mutableStateOf(false) }
    var editPassword by rememberSaveable { mutableStateOf(false) }
    var confirmSignOut by rememberSaveable { mutableStateOf(false) }
    var confirmSwitchServer by rememberSaveable { mutableStateOf(false) }
    var confirmFreeUp by rememberSaveable { mutableStateOf(false) }

    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp).padding(top = 24.dp, bottom = 32.dp)) {
            Text("Settings", style = MaterialTheme.typography.headlineLarge, modifier = Modifier.semantics { heading() })
            Spacer(Modifier.height(20.dp))
            ProfileHeader(controller, catalog, onEdit = { editName = true })

            SettingsGroup("Account") {
                SettingsRow(BrandIcons.User, "Display name", controller.displayName.ifBlank { "Not set" }, onClick = { editName = true })
                SettingsRow(BrandIcons.Lock, "Password", "Change the password you sign in with", onClick = { editPassword = true })
                SettingsRow(BrandIcons.Server, "Server", controller.serverUrl.removePrefix("https://").removePrefix("http://"), onClick = { confirmSwitchServer = true }, trailing = { RowAction("Change") })
            }

            SettingsGroup("On this device") {
                NotificationsRow(controller)
                StorageRow(controller, catalog, onFreeUp = { confirmFreeUp = true })
            }

            SettingsGroup("About") {
                UpdateRow()
                SettingsRow(BrandIcons.External, "Source code", GITHUB_URL.removePrefix("https://"), onClick = { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(GITHUB_URL))) })
            }

            SettingsGroup(null) {
                SettingsRow(BrandIcons.SignOut, "Sign out", null, tint = MaterialTheme.colorScheme.error, onClick = { confirmSignOut = true })
            }
        }
    }

    if (editName) NameDialog(controller, onDismiss = { editName = false })
    if (editPassword) PasswordDialog(controller, onDismiss = { editPassword = false })
    if (confirmSignOut) ConfirmDialog(
        "Sign out?", "You'll need your email and password to sign back in.", "Sign out", destructive = true,
        onConfirm = { confirmSignOut = false; controller.signOut() }, onDismiss = { confirmSignOut = false },
    )
    if (confirmSwitchServer) ConfirmDialog(
        "Change server?", "You'll be signed out of ${controller.serverUrl} and asked for a new server address.", "Change server", destructive = true,
        onConfirm = { confirmSwitchServer = false; controller.switchServer() }, onDismiss = { confirmSwitchServer = false },
    )
    if (confirmFreeUp) {
        val finished = finishedDownloads(catalog)
        ConfirmDialog(
            "Remove finished downloads?",
            "$finished finished ${if (finished == 1) "book is" else "books are"} deleted from this device. Your progress, highlights, and the server's copies stay, and you can download them again any time.",
            "Remove", destructive = false,
            onConfirm = { confirmFreeUp = false; controller.removeFinishedDownloads() }, onDismiss = { confirmFreeUp = false },
        )
    }
}

@Composable
private fun ProfileHeader(controller: AppController, catalog: LibraryUiState.Catalog, onEdit: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(onClickLabel = "Edit your name", role = Role.Button, onClick = onEdit).padding(18.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(56.dp).clip(CircleShape).background(MaterialTheme.colorScheme.secondary), contentAlignment = Alignment.Center) {
            Text(initialsOf(controller.displayName), style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.onSecondary)
        }
        Column(Modifier.weight(1f).padding(start = 16.dp)) {
            Text(controller.displayName.ifBlank { "Signed in" }, style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(catalog.instanceName, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Icon(BrandIcons.ChevronRight, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** A titled, rounded group of rows separated by inset hairlines. */
@Composable
private fun SettingsGroup(title: String?, content: @Composable ColumnScope.() -> Unit) {
    if (title != null) {
        Text(
            title.uppercase(), Modifier.padding(start = 4.dp, top = 28.dp, bottom = 10.dp).semantics { heading() },
            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.4.sp,
        )
    } else Spacer(Modifier.height(28.dp))
    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(MaterialTheme.colorScheme.surfaceVariant).animateContentSize(), content = content)
}

@Composable
private fun SettingsRow(
    icon: ImageVector,
    title: String,
    subtitle: String?,
    tint: Color = MaterialTheme.colorScheme.onSurface,
    onClick: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
    trailing: (@Composable () -> Unit)? = if (onClick != null) { { Icon(BrandIcons.ChevronRight, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) } } else null,
) {
    Row(
        modifier.fillMaxWidth().heightIn(min = 64.dp).then(if (onClick != null) Modifier.clickable(role = Role.Button, onClick = onClick) else Modifier).padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(36.dp).clip(RoundedCornerShape(10.dp)).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f)), contentAlignment = Alignment.Center) {
            Icon(icon, null, Modifier.size(20.dp), tint = if (tint == MaterialTheme.colorScheme.onSurface) MaterialTheme.colorScheme.secondary else tint)
        }
        Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, color = tint)
            if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
        trailing?.invoke()
    }
}

@Composable
private fun RowAction(label: String) = Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)

@Composable
private fun NotificationsRow(controller: AppController) {
    val context = LocalContext.current
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> controller.setNotifications(granted) }
    SettingsRow(
        BrandIcons.Bell, "Notifications", "New books, fulfilled requests, and friend requests",
        modifier = Modifier.toggleable(value = controller.notificationsEnabled, role = Role.Switch) { on ->
            val needsPermission = on && Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            if (needsPermission) permission.launch(Manifest.permission.POST_NOTIFICATIONS) else controller.setNotifications(on)
        },
        trailing = { Switch(checked = controller.notificationsEnabled, onCheckedChange = null) },
    )
    HorizontalDivider(Modifier.padding(start = 66.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))
}

private fun finishedDownloads(catalog: LibraryUiState.Catalog) =
    catalog.books.count { book -> isFinished(catalog.progress[book.id]) && book.editions.any { catalog.downloads[it.id] == DownloadStatus.AVAILABLE } }

/** Space used by downloads, with a usage bar and a one-tap way to free what's already been read. */
@Composable
private fun StorageRow(controller: AppController, catalog: LibraryUiState.Catalog, onFreeUp: () -> Unit) {
    val bytes by produceState(0L, catalog.downloads) { value = withContext(Dispatchers.IO) { controller.downloadedBytes() } }
    val count = catalog.downloads.count { it.value == DownloadStatus.AVAILABLE }
    val finished = finishedDownloads(catalog)
    val downloadable = catalog.books.size.coerceAtLeast(1)
    val share by animateFloatAsState(count.toFloat() / downloadable, tween(500), label = "storage")
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(36.dp).clip(RoundedCornerShape(10.dp)).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f)), contentAlignment = Alignment.Center) {
                Icon(BrandIcons.Storage, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.secondary)
            }
            Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                Text("Downloads", style = MaterialTheme.typography.titleMedium)
                Text(
                    if (count == 0) "Nothing downloaded yet" else "$count ${if (count == 1) "book" else "books"} · ${formatBytes(bytes)}",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (finished > 0) TextButton(onClick = onFreeUp) { Text("Free up", color = cautionColor()) }
        }
        Box(Modifier.padding(start = 50.dp, top = 10.dp).fillMaxWidth().height(6.dp).clip(CircleShape).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f))) {
            Box(Modifier.fillMaxWidth(share.coerceIn(0f, 1f)).fillMaxHeight().clip(CircleShape).background(MaterialTheme.colorScheme.secondary))
        }
        if (finished > 0) Text(
            "$finished finished ${if (finished == 1) "book is" else "books are"} still downloaded",
            Modifier.padding(start = 50.dp, top = 6.dp), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private sealed interface UpdateStatus {
    data object Idle : UpdateStatus
    data object Checking : UpdateStatus
    data object UpToDate : UpdateStatus
    data class Available(val release: Release) : UpdateStatus
    data object Failed : UpdateStatus
}

@Composable
private fun UpdateRow() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val installed = remember { context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty() }
    var status by remember { mutableStateOf<UpdateStatus>(UpdateStatus.Idle) }
    val available = status as? UpdateStatus.Available
    SettingsRow(
        BrandIcons.Info, "BookHarbor $installed",
        when (val s = status) {
            UpdateStatus.Idle -> "Tap to check for updates"
            UpdateStatus.Checking -> "Checking for updates…"
            UpdateStatus.UpToDate -> "You're on the latest version"
            is UpdateStatus.Available -> "Version ${s.release.version} is available"
            UpdateStatus.Failed -> "Couldn't check. Try again when you're online."
        },
        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
        onClick = {
            if (available != null) context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(available.release.downloadUrl)))
            else if (status != UpdateStatus.Checking) {
                status = UpdateStatus.Checking
                scope.launch {
                    status = try {
                        val latest = withContext(Dispatchers.IO) { latestRelease() }
                        if (isNewer(latest.version, installed)) UpdateStatus.Available(latest) else UpdateStatus.UpToDate
                    } catch (_: Exception) { UpdateStatus.Failed }
                }
            }
        },
        trailing = {
            when (status) {
                UpdateStatus.Checking -> CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.secondary)
                is UpdateStatus.Available -> RowAction("Download")
                else -> RowAction("Check")
            }
        },
    )
    HorizontalDivider(Modifier.padding(start = 66.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))
}

@Composable
private fun NameDialog(controller: AppController, onDismiss: () -> Unit) {
    val state = controller.profileUi
    var name by rememberSaveable { mutableStateOf(controller.displayName) }
    LaunchedEffect(Unit) { controller.resetProfileStatus() }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Display name") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Friends see this name.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(name, { name = it }, singleLine = true, shape = ControlShape, modifier = Modifier.fillMaxWidth())
                state.error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            }
        },
        confirmButton = {
            TextButton(onClick = { controller.updateDisplayName(name.trim(), onSuccess = onDismiss) }, enabled = !state.saving && name.isNotBlank() && name.trim() != controller.displayName) {
                Text(if (state.saving) "Saving…" else "Save")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun PasswordDialog(controller: AppController, onDismiss: () -> Unit) {
    val state = controller.profileUi
    var current by rememberSaveable { mutableStateOf("") }
    var next by rememberSaveable { mutableStateOf("") }
    var confirm by rememberSaveable { mutableStateOf("") }
    LaunchedEffect(Unit) { controller.resetProfileStatus() }
    LaunchedEffect(state.passwordChanged) { if (state.passwordChanged) { controller.notice = "Password updated"; onDismiss() } }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Change password") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                val field = @Composable { value: String, onChange: (String) -> Unit, label: String, error: Boolean ->
                    OutlinedTextField(
                        value, onChange, label = { Text(label) }, singleLine = true, isError = error, shape = ControlShape, modifier = Modifier.fillMaxWidth(),
                        visualTransformation = PasswordVisualTransformation(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                    )
                }
                field(current, { current = it }, "Current password", false)
                field(next, { next = it }, "New password", next.isNotEmpty() && next.length < 12)
                field(confirm, { confirm = it }, "Confirm new password", confirm.isNotEmpty() && confirm != next)
                Text(
                    state.error ?: "At least 12 characters. Other devices stay signed in.",
                    style = MaterialTheme.typography.bodySmall, color = if (state.error != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { controller.changePassword(current, next) }, enabled = !state.saving && current.isNotBlank() && next.length >= 12 && next == confirm) {
                Text(if (state.saving) "Saving…" else "Update")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ConfirmDialog(title: String, text: String, action: String, destructive: Boolean, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(text) },
        confirmButton = { TextButton(onClick = onConfirm) { Text(action, color = if (destructive) MaterialTheme.colorScheme.error else cautionColor()) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
