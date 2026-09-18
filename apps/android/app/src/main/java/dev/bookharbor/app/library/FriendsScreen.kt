package dev.bookharbor.app.library

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import java.time.Year

/** Friends: send/accept requests, see who's sharing what they're reading, and control your own sharing. */
@Composable
fun FriendsTab(controller: AppController) {
    LaunchedEffect(Unit) { controller.loadFriends() }
    val state = controller.friendsUi

    PullToRefreshBox(isRefreshing = state.loading, onRefresh = controller::loadFriends, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp)) {
        item { Text("Friends", style = MaterialTheme.typography.headlineLarge) }
        item { AddFriendRow(onSend = controller::sendFriendRequest) }

        if (state.error != null) item {
            Text(state.error, Modifier.padding(top = 12.dp), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
        }

        if (state.incoming.isNotEmpty() || state.outgoing.isNotEmpty()) {
            item { SectionLabel("Requests") }
            items(state.incoming, key = { "in_" + it.userId }) { request ->
                RequestRow(request, primaryLabel = "Accept", onPrimary = { controller.acceptFriendRequest(request.userId) }, onDismiss = { controller.declineFriendRequest(request.userId) }, dismissLabel = "Decline")
            }
            items(state.outgoing, key = { "out_" + it.userId }) { request ->
                RequestRow(request, primaryLabel = null, onPrimary = null, onDismiss = { controller.cancelFriendRequest(request.userId) }, dismissLabel = "Cancel")
            }
        }

        item { SectionLabel("Your friends") }
        if (state.friends.isEmpty() && !state.loading) item {
            Text("No friends yet. Add one by email above.", Modifier.padding(vertical = 12.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        items(state.friends, key = { it.userId }) { friend ->
            FriendCard(friend, onRemove = { controller.removeFriend(friend.userId) })
        }

        item { SectionLabel("Sharing") }
        item { SharingSettings(state.settings, onChange = controller::updateSocialSettings) }
    }
    }
}

@Composable
private fun SectionLabel(label: String) {
    Text(label, Modifier.padding(top = 24.dp, bottom = 8.dp), style = MaterialTheme.typography.titleMedium)
}

@Composable
private fun AddFriendRow(onSend: (String) -> Unit) {
    var email by rememberSaveable { mutableStateOf("") }
    Row(Modifier.padding(top = 14.dp), verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = email, onValueChange = { email = it }, singleLine = true, modifier = Modifier.weight(1f),
            label = { Text("Add a friend by email") }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email), shape = RoundedCornerShape(12.dp),
        )
        Button(onClick = { if (email.isNotBlank()) { onSend(email.trim()); email = "" } }, Modifier.padding(start = 10.dp), enabled = email.isNotBlank(), shape = RoundedCornerShape(9.dp)) {
            Text("Send")
        }
    }
}

@Composable
private fun RequestRow(request: FriendRequest, primaryLabel: String?, onPrimary: (() -> Unit)?, onDismiss: () -> Unit, dismissLabel: String) {
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        Avatar(request.displayName)
        Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
            Text(request.displayName, style = MaterialTheme.typography.titleSmall)
            Text(if (primaryLabel == null) "Pending" else "Wants to be friends", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (primaryLabel != null && onPrimary != null) TextButton(onClick = onPrimary) { Text(primaryLabel) }
        TextButton(onClick = onDismiss) { Text(dismissLabel) }
    }
}

@Composable
private fun FriendCard(friend: Friend, onRemove: () -> Unit) {
    Column(Modifier.fillMaxWidth().padding(vertical = 10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Avatar(friend.displayName)
            Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                Text(friend.displayName, style = MaterialTheme.typography.titleSmall)
                if (friend.finishedThisYear != null) {
                    val goal = friend.goalBooks
                    Text(
                        if (goal != null) "Finished ${friend.finishedThisYear} this year · Goal $goal" else "Finished ${friend.finishedThisYear} this year",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Text("Activity is private", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            TextButton(onClick = onRemove) { Text("Remove") }
        }
        friend.currentlyReading.forEach { book ->
            Column(Modifier.padding(start = 52.dp, top = 6.dp)) {
                Text(book.title, style = MaterialTheme.typography.bodyMedium)
                LinearProgressIndicator(progress = { book.percentage.toFloat() }, modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
            }
        }
    }
}

@Composable
private fun Avatar(name: String) {
    Box(Modifier.size(40.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant), contentAlignment = Alignment.Center) {
        Text(initialsOf(name), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun SharingSettings(settings: SocialSettings, onChange: (Boolean, Int, Int) -> Unit) {
    var goalText by remember(settings.goalBooks) { mutableStateOf(if (settings.goalBooks > 0) settings.goalBooks.toString() else "") }
    Column {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("Share my reading activity with friends", style = MaterialTheme.typography.bodyMedium)
                Text("Friends can see what you're reading and how far you've gotten.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Switch(
                checked = settings.activityVisible,
                onCheckedChange = { visible -> onChange(visible, settings.goalYear, settings.goalBooks) },
            )
        }
        Row(Modifier.fillMaxWidth().padding(top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                value = goalText, onValueChange = { goalText = it.filter(Char::isDigit) }, singleLine = true, modifier = Modifier.weight(1f),
                label = { Text("Books to read this year") }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), shape = RoundedCornerShape(12.dp),
            )
            Button(
                onClick = {
                    val books = goalText.toIntOrNull() ?: 0
                    onChange(settings.activityVisible, if (books > 0) Year.now().value else 0, books)
                },
                Modifier.padding(start = 10.dp), shape = RoundedCornerShape(9.dp),
            ) { Text("Save") }
        }
    }
}
