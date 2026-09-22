package dev.bookharbor.app.library

import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import dev.bookharbor.app.ui.AppRow
import dev.bookharbor.app.ui.AppTextField
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.EmptyState
import dev.bookharbor.app.ui.GroupCard
import dev.bookharbor.app.ui.Motion
import dev.bookharbor.app.ui.RowAction
import dev.bookharbor.app.ui.RowDivider
import dev.bookharbor.app.ui.ScreenTitle
import dev.bookharbor.app.ui.SecondaryButton
import dev.bookharbor.app.ui.SectionHeader
import java.text.DateFormat
import java.time.Instant
import java.time.Year
import java.util.Date

/** Friends: your own reading at the top, then requests, then friends -- each opening their page. */
@Composable
fun FriendsTab(controller: AppController) {
    var openFriend by rememberSaveable { mutableStateOf<String?>(null) }
    openFriend?.let { id ->
        BackHandler { openFriend = null }
        FriendProfileScreen(controller, id, onBack = { openFriend = null })
        return
    }
    LaunchedEffect(Unit) { controller.loadFriends() }
    val state = controller.friendsUi
    var adding by rememberSaveable { mutableStateOf(false) }
    var editingGoal by rememberSaveable { mutableStateOf(false) }

    PullToRefreshBox(isRefreshing = state.loading, onRefresh = controller::loadFriends, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 32.dp)) {
            item {
                ScreenTitle("Friends") {
                    IconButton(onClick = { adding = true }) { Icon(BrandIcons.UserPlus, "Add a friend", tint = MaterialTheme.colorScheme.secondary) }
                }
            }
            item { Box(Modifier.padding(top = 16.dp)) { YourReadingCard(controller, state.settings, onEditGoal = { editingGoal = true }) } }
            if (state.error != null) item {
                Text(state.error, Modifier.padding(top = 12.dp), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }

            if (state.incoming.isNotEmpty() || state.outgoing.isNotEmpty()) {
                item { SectionHeader("Requests") }
                item {
                    GroupCard {
                        state.incoming.forEachIndexed { index, request ->
                            AppRow(
                                request.displayName, "Wants to be friends", leading = { Avatar(request.displayName) },
                                trailing = {
                                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                        TextButton(onClick = { controller.declineFriendRequest(request.userId) }) { Text("Decline", color = MaterialTheme.colorScheme.onSurfaceVariant) }
                                        TextButton(onClick = { controller.acceptFriendRequest(request.userId) }) { Text("Accept") }
                                    }
                                },
                            )
                            if (index < state.incoming.lastIndex || state.outgoing.isNotEmpty()) RowDivider()
                        }
                        state.outgoing.forEachIndexed { index, request ->
                            AppRow(
                                request.displayName, "Request sent", leading = { Avatar(request.displayName) },
                                trailing = { TextButton(onClick = { controller.cancelFriendRequest(request.userId) }) { Text("Cancel", color = MaterialTheme.colorScheme.onSurfaceVariant) } },
                            )
                            if (index < state.outgoing.lastIndex) RowDivider()
                        }
                    }
                }
            }

            item { SectionHeader("Your friends") }
            if (state.friends.isEmpty() && !state.loading) item {
                EmptyState(BrandIcons.Friends, "No friends yet", "Add someone on this server by their email to see what they're reading.", action = {
                    SecondaryButton("Add a friend", onClick = { adding = true }, icon = BrandIcons.Friends)
                })
            } else item {
                GroupCard {
                    state.friends.forEachIndexed { index, friend ->
                        AppRow(
                            friend.displayName, friendSummary(friend), leading = { Avatar(friend.displayName) },
                            onClick = { controller.loadFriendProfile(friend.userId); openFriend = friend.userId },
                        )
                        if (index < state.friends.lastIndex) RowDivider()
                    }
                }
            }
        }
    }

    if (adding) AddFriendDialog(controller, onDismiss = { adding = false })
    if (editingGoal) GoalDialog(state.settings, onSave = { books -> controller.updateSocialSettings(state.settings.activityVisible, if (books > 0) Year.now().value else 0, books); editingGoal = false }, onDismiss = { editingGoal = false })
}

private fun friendSummary(friend: Friend): String = when {
    !friend.activityVisible -> "Keeps their reading private"
    friend.currentlyReading.isNotEmpty() -> "Reading ${friend.currentlyReading.first().title}" + if (friend.currentlyReading.size > 1) " and ${friend.currentlyReading.size - 1} more" else ""
    friend.finishedThisYear != null -> "Finished ${friend.finishedThisYear} this year"
    else -> "Not reading anything right now"
}

/** This year's count, the goal as a bar, and whether friends can see it. */
@Composable
private fun YourReadingCard(controller: AppController, settings: SocialSettings, onEditGoal: () -> Unit) {
    val hasGoal = settings.goalBooks > 0 && settings.goalYear == Year.now().value
    val fraction by animateFloatAsState(if (hasGoal) (settings.finishedThisYear.toFloat() / settings.goalBooks).coerceIn(0f, 1f) else 0f, Motion.standard(), label = "goal")
    GroupCard {
        Column(Modifier.padding(18.dp)) {
            Text("YOUR READING IN ${Year.now().value}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
            Row(Modifier.padding(top = 6.dp), verticalAlignment = Alignment.Bottom) {
                Text("${settings.finishedThisYear}", style = MaterialTheme.typography.displaySmall)
                Text(
                    if (hasGoal) "  of ${settings.goalBooks} books" else "  ${if (settings.finishedThisYear == 1) "book" else "books"} finished",
                    Modifier.padding(bottom = 6.dp), style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (hasGoal) Box(Modifier.padding(top = 10.dp).fillMaxWidth().height(8.dp).clip(CircleShape).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f))) {
                Box(Modifier.fillMaxWidth(fraction).fillMaxHeight().clip(CircleShape).background(MaterialTheme.colorScheme.secondary))
            }
        }
        RowDivider(inset = 0.dp)
        AppRow("Reading goal", if (hasGoal) "${settings.goalBooks} books this year" else "Not set", icon = BrandIcons.Check, onClick = onEditGoal, trailing = { RowAction(if (hasGoal) "Change" else "Set") })
        RowDivider()
        SharingRow(controller, settings)
    }
}

@Composable
private fun SharingRow(controller: AppController, settings: SocialSettings) {
    AppRow(
        "Share my reading", "Friends see what you're reading and what you've finished",
        modifier = Modifier.toggleable(value = settings.activityVisible, role = Role.Switch) { controller.updateSocialSettings(it, settings.goalYear, settings.goalBooks) },
        icon = BrandIcons.Friends,
        trailing = { Switch(checked = settings.activityVisible, onCheckedChange = null) },
    )
}

@Composable
private fun AddFriendDialog(controller: AppController, onDismiss: () -> Unit) {
    var email by rememberSaveable { mutableStateOf("") }
    val error = controller.friendsUi.error
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add a friend") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("They'll get a request, and once they accept you'll see each other's reading if you both share it.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                AppTextField(email, { email = it }, "Their email", keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email))
                if (error != null) Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        },
        // Closes only once the request succeeds, so a typo stays in place to fix.
        confirmButton = { TextButton(onClick = { controller.sendFriendRequest(email.trim()) { onDismiss(); controller.notice = "Request sent" } }, enabled = email.isNotBlank()) { Text("Send request") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun GoalDialog(settings: SocialSettings, onSave: (Int) -> Unit, onDismiss: () -> Unit) {
    var value by rememberSaveable { mutableStateOf(if (settings.goalBooks > 0) settings.goalBooks.toString() else "") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Reading goal for ${Year.now().value}") },
        text = { AppTextField(value, { value = it.filter(Char::isDigit).take(4) }, "Books", keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), supporting = "Leave empty for no goal") },
        confirmButton = { TextButton(onClick = { onSave(value.toIntOrNull() ?: 0) }) { Text("Save") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** A friend's page: stats, what they're reading, what they've finished, and removing them. */
@Composable
private fun FriendProfileScreen(controller: AppController, userId: String, onBack: () -> Unit) {
    LaunchedEffect(userId) { controller.loadFriendProfile(userId) }
    val profile = controller.friendProfile?.takeIf { it.friend.userId == userId }
    val catalog = controller.ui as? LibraryUiState.Catalog
    var confirmRemove by rememberSaveable { mutableStateOf(false) }
    LazyColumn(Modifier.fillMaxSize().statusBarsPadding(), contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 32.dp)) {
        item {
            Row(Modifier.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack, modifier = Modifier.offset(x = (-12).dp)) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to Friends") }
            }
        }
        if (profile == null) {
            item { Box(Modifier.fillMaxWidth().padding(48.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator(color = MaterialTheme.colorScheme.secondary) } }
            return@LazyColumn
        }
        val friend = profile.friend
        item {
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
                Avatar(friend.displayName, size = 88.dp)
                Text(friend.displayName, Modifier.padding(top = 14.dp), style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center)
                Text(friend.email, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (!friend.activityVisible) {
            item { EmptyState(BrandIcons.Lock, "Reading kept private", "${friend.displayName} hasn't chosen to share what they read. If they turn sharing on, it will show here.") }
        } else {
            item {
                Row(Modifier.fillMaxWidth().padding(top = 24.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Stat("Finished", profile.finishedTotal ?: 0, Modifier.weight(1f))
                    Stat("This year", friend.finishedThisYear ?: 0, Modifier.weight(1f))
                    Stat("In common", profile.booksInCommon ?: 0, Modifier.weight(1f))
                }
            }
            friend.goalBooks?.let { goal ->
                item {
                    val done = friend.finishedThisYear ?: 0
                    GroupCard(Modifier.padding(top = 10.dp)) {
                        Column(Modifier.padding(16.dp)) {
                            Text("${Year.now().value} goal: $done of $goal books", style = MaterialTheme.typography.titleMedium)
                            Box(Modifier.padding(top = 10.dp).fillMaxWidth().height(8.dp).clip(CircleShape).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f))) {
                                Box(Modifier.fillMaxWidth((done.toFloat() / goal).coerceIn(0f, 1f)).fillMaxHeight().clip(CircleShape).background(MaterialTheme.colorScheme.secondary))
                            }
                        }
                    }
                }
            }
            if (profile.readingNow.isNotEmpty()) {
                item { SectionHeader("Reading now") }
                item {
                    LazyRow(Modifier.bleed(20.dp), contentPadding = PaddingValues(horizontal = 20.dp), horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                        items(profile.readingNow, key = { it.bookId }) { book -> ActivityCover(controller, catalog, book.bookId, book.title, book.coverUrl, book.percentage) }
                    }
                }
            }
            item { SectionHeader("Finished") }
            if (profile.finished.isEmpty()) item {
                Text("Nothing finished yet.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            } else item {
                GroupCard {
                    profile.finished.forEachIndexed { index, book ->
                        val onShelf = catalog?.books?.firstOrNull { it.id == book.bookId }
                        AppRow(
                            book.title, finishedDate(book.finishedAt)?.let { "Finished $it" },
                            leading = { Cover(onShelf ?: Book(book.bookId, book.title, emptyList(), coverUrl = book.coverUrl), controller.covers, Modifier.width(40.dp)) },
                        )
                        if (index < profile.finished.lastIndex) RowDivider(inset = 70.dp)
                    }
                }
            }
        }
        item {
            GroupCard(Modifier.padding(top = 28.dp)) {
                AppRow("Remove friend", null, icon = BrandIcons.Close, titleColor = MaterialTheme.colorScheme.error, onClick = { confirmRemove = true }, trailing = null)
            }
        }
    }
    if (confirmRemove && profile != null) AlertDialog(
        onDismissRequest = { confirmRemove = false },
        title = { Text("Remove ${profile.friend.displayName}?") },
        text = { Text("You'll stop seeing each other's reading. To be friends again, one of you will need to send a new request.") },
        confirmButton = { TextButton(onClick = { confirmRemove = false; controller.removeFriend(userId); onBack() }) { Text("Remove", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = { confirmRemove = false }) { Text("Cancel") } },
    )
}

@Composable
private fun Stat(label: String, value: Int, modifier: Modifier) {
    GroupCard(modifier) {
        Column(Modifier.fillMaxWidth().padding(vertical = 14.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Text("$value", style = MaterialTheme.typography.headlineMedium)
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** A cover from a friend's reading, with their progress; tapping it shows the book in your library. */
@Composable
private fun ActivityCover(controller: AppController, catalog: LibraryUiState.Catalog?, bookId: String, title: String, coverUrl: String, percentage: Double) {
    val book = catalog?.books?.firstOrNull { it.id == bookId }
    Column(Modifier.width(104.dp)) {
        Cover(book ?: Book(bookId, title, emptyList(), coverUrl = coverUrl), controller.covers, Modifier.fillMaxWidth().clickable(enabled = book != null, onClickLabel = "Details for $title", role = Role.Button) { book?.let(controller::showDetails) })
        Box(Modifier.padding(top = 8.dp).fillMaxWidth().height(4.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant)) {
            Box(Modifier.fillMaxWidth(percentage.toFloat().coerceIn(0f, 1f)).fillMaxHeight().clip(CircleShape).background(MaterialTheme.colorScheme.secondary))
        }
        Text(title, Modifier.padding(top = 6.dp), style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
        Text("${(percentage * 100).toInt()}%", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

private fun finishedDate(iso: String): String? =
    runCatching { DateFormat.getDateInstance(DateFormat.MEDIUM).format(Date.from(Instant.parse(iso))) }.getOrNull()

@Composable
internal fun Avatar(name: String, size: Dp = 40.dp) {
    Box(Modifier.size(size).clip(CircleShape).background(MaterialTheme.colorScheme.secondary), contentAlignment = Alignment.Center) {
        Text(initialsOf(name), style = if (size > 60.dp) MaterialTheme.typography.headlineMedium else MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSecondary)
    }
}
