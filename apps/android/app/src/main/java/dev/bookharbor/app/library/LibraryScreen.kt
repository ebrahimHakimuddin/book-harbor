package dev.bookharbor.app.library

import android.content.Intent
import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Switch
import androidx.core.content.ContextCompat
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.ui.semantics.heading
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.ui.text.input.ImeAction
import dev.bookharbor.app.ui.AppTextField
import dev.bookharbor.app.ui.PrimaryButton
import androidx.compose.foundation.BorderStroke
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.border
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.layout
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.material3.ripple
import androidx.compose.ui.composed
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import dev.bookharbor.app.ui.pressScale
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyGridItemSpanScope
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import kotlinx.coroutines.launch
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.bookharbor.app.R
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.theme.HarborNavy
import dev.bookharbor.app.ui.theme.LiterataFamily
import dev.bookharbor.app.ui.theme.cautionColor
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.text.DateFormat
import java.util.Date
import kotlin.math.roundToInt

private enum class Tab(val label: String) { Home("Home"), Browse("Browse"), Friends("Friends"), Requests("Requests"), Settings("Settings") }

@Composable
fun LibraryScreen(controller: AppController) {
    when (val state = controller.ui) {
        LibraryUiState.Setup -> EntryColumn(tagline = "Connect to your BookHarbor server to begin.") {
            ServerForm(initial = controller.serverUrl, onConnect = controller::connect)
        }
        is LibraryUiState.SignIn -> EntryColumn(title = state.instance?.name, tagline = "Sign in to your library.") {
            var changingServer by rememberSaveable { mutableStateOf(false) }
            var resetting by rememberSaveable { mutableStateOf(false) }
            ServerPill(state.serverUrl) { changingServer = true }
            SignInForm(initialEmail = state.email, onSignIn = controller::signIn)
            if (state.instance?.passwordResetEnabled == true) TextButton(onClick = { resetting = true }) { Text("Forgot password?") }
            if (resetting) PasswordResetDialog(controller, state.email, onDismiss = { resetting = false; controller.dismissPasswordReset() })
            if (changingServer) ChangeServerDialog(state.serverUrl, onConnect = { changingServer = false; controller.connect(it) }, onDismiss = { changingServer = false })
        }
        LibraryUiState.Loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(72.dp))
                CircularProgressIndicator(color = MaterialTheme.colorScheme.secondary)
                Text("Opening your library…", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        is LibraryUiState.Error -> EntryColumn(tagline = null) {
            Text(state.message, style = MaterialTheme.typography.bodyLarge, textAlign = TextAlign.Center, color = MaterialTheme.colorScheme.error)
            PrimaryButton("Try again", onClick = { if (state.retry == LibraryUiState.Loading) controller.load() else controller.dismissError(state.retry) })
        }
        is LibraryUiState.Catalog -> CatalogScaffold(controller, state)
    }
}

/** The brand column used before you are signed in: mark, name, a line of context, then the form. */
@Composable
private fun EntryColumn(title: String? = null, tagline: String?, content: @Composable () -> Unit) {
    Column(
        // imePadding: the app draws edge to edge, so the form must make room for the keyboard itself.
        Modifier.fillMaxSize().statusBarsPadding().imePadding().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Spacer(Modifier.height(32.dp))
        Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(84.dp))
        Text(title ?: "BookHarbor", style = MaterialTheme.typography.headlineLarge, color = MaterialTheme.colorScheme.onBackground, textAlign = TextAlign.Center)
        if (tagline != null) Text(tagline, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
        Spacer(Modifier.height(12.dp))
        content()
    }
}

/**
 * Asks before connecting over plain http: home servers often use it, so it's allowed, but the
 * reader decides knowing what it means.
 */
@Composable
private fun rememberHttpGuard(onConnect: (String) -> Unit): (String) -> Unit {
    var pending by remember { mutableStateOf<String?>(null) }
    pending?.let { url ->
        AlertDialog(
            onDismissRequest = { pending = null },
            icon = { Icon(BrandIcons.Lock, null, tint = cautionColor()) },
            title = { Text("This connection isn't encrypted") },
            text = { Text("Anyone on this network could see your password and books. Only continue at home or on a network you trust.") },
            confirmButton = { TextButton(onClick = { pending = null; onConnect(url) }) { Text("Connect anyway", color = cautionColor()) } },
            dismissButton = { TextButton(onClick = { pending = null }) { Text("Cancel") } },
        )
    }
    return { url -> if (url.trim().startsWith("http://", ignoreCase = true)) pending = url.trim() else onConnect(url.trim()) }
}

@Composable
private fun ServerForm(initial: String, onConnect: (String) -> Unit) {
    var value by remember(initial) { mutableStateOf(initial) }
    val connect = rememberHttpGuard(onConnect)
    AppTextField(
        value, { value = it }, "Server address", placeholder = "https://books.example.com",
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go),
        keyboardActions = KeyboardActions(onGo = { if (value.isNotBlank()) connect(value) }),
    )
    PrimaryButton("Connect", onClick = { connect(value) }, enabled = value.isNotBlank())
}

/** Which server you're signing in to, tappable to change it without leaving this screen. */
@Composable
private fun ServerPill(serverUrl: String, onChange: () -> Unit) {
    val secure = serverUrl.startsWith("https://", ignoreCase = true)
    Row(
        Modifier.clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(onClickLabel = "Change server", role = Role.Button, onClick = onChange).padding(start = 14.dp, end = 10.dp, top = 8.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(if (secure) BrandIcons.Lock else BrandIcons.Server, if (secure) "Encrypted" else null, Modifier.size(16.dp), tint = MaterialTheme.colorScheme.secondary)
        Text(serverUrl.removePrefix("https://").removePrefix("http://"), Modifier.padding(horizontal = 8.dp), style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        Text("Change", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)
    }
}

@Composable
private fun ChangeServerDialog(current: String, onConnect: (String) -> Unit, onDismiss: () -> Unit) {
    var value by rememberSaveable { mutableStateOf(current) }
    val connect = rememberHttpGuard(onConnect)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Change server") },
        text = {
            AppTextField(
                value, { value = it }, "Server address", placeholder = "https://books.example.com",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go),
                keyboardActions = KeyboardActions(onGo = { if (value.isNotBlank()) connect(value) }),
            )
        },
        confirmButton = { TextButton(onClick = { connect(value) }, enabled = value.isNotBlank() && value.trim() != current) { Text("Connect") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun SignInForm(initialEmail: String, onSignIn: (String, String) -> Unit) {
    var email by remember(initialEmail) { mutableStateOf(initialEmail) }
    var password by remember { mutableStateOf("") }
    val ready = email.isNotBlank() && password.isNotBlank()
    AppTextField(email, { email = it }, "Email", keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, imeAction = ImeAction.Next))
    AppTextField(
        password, { password = it }, "Password", visualTransformation = PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
        keyboardActions = KeyboardActions(onGo = { if (ready) onSignIn(email.trim(), password) }),
    )
    PrimaryButton("Sign in", onClick = { onSignIn(email.trim(), password) }, enabled = ready)
}

/** Ask for an emailed code, then set a new password with it. The server never says whether an address has an account. */
@Composable
private fun PasswordResetDialog(controller: AppController, initialEmail: String, onDismiss: () -> Unit) {
    val state = controller.passwordResetUi
    var email by rememberSaveable { mutableStateOf(initialEmail) }
    var code by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (state.done) "Password changed" else "Reset your password") },
        text = {
            Column(Modifier.animateContentSize(), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                when {
                    state.done -> Text("Sign in with your new password. Every device that was signed in has been signed out.")
                    !state.codeSent -> {
                        Text("We'll email you a code to choose a new password.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                        AppTextField(email, { email = it }, "Email", keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email))
                    }
                    else -> {
                        Text("If $email has an account, a code is on its way. It expires in 30 minutes.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                        AppTextField(code, { code = it.uppercase().filter { c -> c.isLetterOrDigit() }.take(10) }, "Code", keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters))
                        AppTextField(password, { password = it }, "New password", supporting = "At least 12 characters", visualTransformation = PasswordVisualTransformation(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password))
                    }
                }
                state.error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            }
        },
        confirmButton = {
            when {
                state.done -> TextButton(onClick = onDismiss) { Text("Sign in") }
                !state.codeSent -> TextButton(onClick = { controller.requestPasswordReset(email.trim()) }, enabled = !state.working && email.isNotBlank()) { Text(if (state.working) "Sending…" else "Send code") }
                else -> TextButton(onClick = { controller.confirmPasswordReset(email.trim(), code, password) }, enabled = !state.working && code.length == 10 && password.length >= 12) { Text(if (state.working) "Saving…" else "Set password") }
            }
        },
        dismissButton = { if (!state.done) TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun CatalogScaffold(controller: AppController, catalog: LibraryUiState.Catalog) {
    var tab by rememberSaveable { mutableStateOf(Tab.Home) }
    var listsOpen by rememberSaveable { mutableStateOf(false) }
    var historyOpen by rememberSaveable { mutableStateOf(false) }
    if (listsOpen) {
        ListsScreen(controller, onClose = { listsOpen = false })
        return
    }
    if (historyOpen) {
        BackHandler { historyOpen = false }
        HistoryTab(controller, catalog, onBack = { historyOpen = false })
        return
    }
    val snackbar = remember { SnackbarHostState() }
    // Ask for the notification permission once, when the library first appears.
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    LaunchedEffect(Unit) { if (Build.VERSION.SDK_INT >= 33 && controller.firstNotificationPrompt()) permission.launch(Manifest.permission.POST_NOTIFICATIONS) }
    LaunchedEffect(controller.notice) {
        controller.notice?.let { snackbar.showSnackbar(it); controller.notice = null }
    }
    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        snackbarHost = { SnackbarHost(snackbar) },
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                Tab.entries.forEach { item ->
                    NavigationBarItem(
                        selected = tab == item,
                        onClick = { tab = item },
                        icon = { Icon(when (item) { Tab.Home -> BrandIcons.Library; Tab.Browse -> BrandIcons.Search; Tab.Friends -> BrandIcons.Friends; Tab.Requests -> BrandIcons.Request; Tab.Settings -> BrandIcons.Settings }, contentDescription = null) },
                        label = { Text(item.label) },
                        colors = NavigationBarItemDefaults.colors(selectedIconColor = MaterialTheme.colorScheme.onPrimary, selectedTextColor = MaterialTheme.colorScheme.primary, indicatorColor = MaterialTheme.colorScheme.primary),
                    )
                }
            }
        },
    ) { padding ->
        // A quick crossfade between tabs rather than a hard cut.
        Crossfade(tab, Modifier.padding(padding).fillMaxSize(), animationSpec = tween(180), label = "tab") { shown ->
            when (shown) {
                Tab.Home -> HomeTab(controller, catalog, onOpenHistory = { historyOpen = true }, onOpenLists = { listsOpen = true }, onOpenSettings = { tab = Tab.Settings }, onBrowse = { tab = Tab.Browse })
                Tab.Browse -> BrowseTab(controller, catalog)
                Tab.Friends -> FriendsTab(controller)
                Tab.Requests -> RequestsTab(controller)
                Tab.Settings -> SettingsTab(controller, catalog)
            }
        }
    }
    controller.unsyncedOnSignOut?.let { count ->
        AlertDialog(
            onDismissRequest = controller::dismissSignOutWarning,
            title = { Text("Some changes haven't synced") },
            text = { Text("$count reading ${if (count == 1) "update or note hasn't" else "updates or notes haven't"} reached your server yet. Signing out now discards ${if (count == 1) "it" else "them"}. Connect to the internet and sync first to keep your place and highlights.") },
            confirmButton = { TextButton(onClick = controller::confirmSignOut) { Text("Sign out anyway", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = controller::dismissSignOutWarning) { Text("Keep me signed in") } },
        )
    }
}


// ---- History ---------------------------------------------------------------------------------

@Composable
private fun HistoryTab(controller: AppController, catalog: LibraryUiState.Catalog, onBack: () -> Unit) {
    val sync = controller.sync
    val synced = sync.pending == 0 && !catalog.offline && sync.error == null
    val read = remember(catalog.books, catalog.lastReadAt) {
        catalog.books.filter { it.id in catalog.lastReadAt }.sortedByDescending { catalog.lastReadAt[it.id] }
    }
    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(horizontal = 20.dp, vertical = 24.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        item {
            Row(Modifier.padding(bottom = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack, modifier = Modifier.offset(x = (-12).dp)) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to Home") }
                Text("Reading history", style = MaterialTheme.typography.headlineMedium, modifier = Modifier.semantics { heading() })
            }
        }
        item {
            Row(
                Modifier.fillMaxWidth().padding(bottom = 18.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant)
                    .clickable(onClick = controller::syncNow, enabled = !sync.running).padding(16.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (sync.running) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.secondary)
                else Icon(if (synced) BrandIcons.CloudDone else if (catalog.offline) BrandIcons.CloudOff else BrandIcons.Cloud, null, Modifier.size(22.dp), tint = MaterialTheme.colorScheme.secondary)
                Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                    Text(
                        when {
                            sync.running -> "Syncing…"
                            catalog.offline -> "You're offline"
                            sync.pending > 0 -> "${sync.pending} ${if (sync.pending == 1) "update" else "updates"} waiting"
                            else -> "Everything is synced"
                        },
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        sync.error ?: if (sync.lastSyncMillis > 0) "Last synced ${DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(sync.lastSyncMillis))}" else "Tap to sync now",
                        style = MaterialTheme.typography.bodySmall, color = if (sync.error != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Icon(BrandIcons.Sync, "Sync now", tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (sync.rejected > 0) item {
            Text(
                "${sync.rejected} reading ${if (sync.rejected == 1) "update was" else "updates were"} refused by the server and won't be retried.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(bottom = 14.dp),
            )
        }
        if (read.isEmpty()) {
            item {
                Column(Modifier.fillMaxWidth().padding(vertical = 48.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Icon(BrandIcons.History, null, Modifier.size(40.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("Nothing read yet", style = MaterialTheme.typography.titleLarge)
                    Text("Books you open will show up here.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
                }
            }
        } else {
            items(read, key = { it.id }) { book -> Box(Modifier.animateItem()) { HistoryRow(controller, catalog, book) } }
        }
    }
    }
}

@Composable
private fun HistoryRow(controller: AppController, catalog: LibraryUiState.Catalog, book: Book) {
    val progress = catalog.progress[book.id]
    val when_ = catalog.lastReadAt[book.id]?.let(::relativeReadTime)
    Row(
        Modifier.fillMaxWidth().clickable(onClickLabel = "Open ${book.title}", role = Role.Button) { controller.open(book) }.padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Cover(book, controller.covers, Modifier.width(52.dp))
        Column(Modifier.weight(1f).padding(horizontal = 14.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(book.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
            val status = when {
                isFinished(progress) -> "Finished"
                isReading(progress) -> "${((progress ?: 0.0) * 100).roundToInt()}% read"
                else -> "Started"
            }
            Text(
                listOfNotNull(status, when_).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A short "3h ago" / "2d ago" / date label from an RFC 3339 UTC instant. */
private fun relativeReadTime(occurredAt: String): String? {
    val then = runCatching { java.time.Instant.parse(occurredAt) }.getOrNull() ?: return null
    val minutes = java.time.Duration.between(then, java.time.Instant.now()).toMinutes()
    return when {
        minutes < 1 -> "Just now"
        minutes < 60 -> "${minutes}m ago"
        minutes < 24 * 60 -> "${minutes / 60}h ago"
        minutes < 7 * 24 * 60 -> "${minutes / (24 * 60)}d ago"
        else -> DateFormat.getDateInstance(DateFormat.MEDIUM).format(Date.from(then))
    }
}
