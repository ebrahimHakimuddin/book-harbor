package dev.bookharbor.app

import android.os.Bundle
import androidx.activity.compose.LocalActivity
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.tooling.preview.Preview
import dev.bookharbor.app.reader.ReaderAction
import dev.bookharbor.app.reader.ReaderScreen
import dev.bookharbor.app.reader.ReaderState
import dev.bookharbor.app.reader.reduce
import dev.bookharbor.app.ui.theme.BookHarborTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            BookHarborApp()
        }
    }
}

@Composable
fun BookHarborApp() {
    var readerState by remember { mutableStateOf(ReaderState.preview()) }
    val activity = LocalActivity.current

    BookHarborTheme(readerTheme = readerState.settings.theme) {
        ReaderScreen(
            state = readerState,
            onClose = { activity?.finish() },
            onAction = { action: ReaderAction ->
                readerState = readerState.reduce(action)
            },
        )
    }
}

@Preview(showBackground = true, widthDp = 393, heightDp = 852)
@Composable
private fun BookHarborPreview() {
    var state by remember { mutableStateOf(ReaderState.preview()) }
    BookHarborTheme(readerTheme = state.settings.theme) {
        ReaderScreen(
            state = state,
            onClose = {},
            onAction = { action -> state = state.reduce(action) },
        )
    }
}
