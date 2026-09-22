package dev.bookharbor.app.ui

import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// The app's shared building blocks. Screens compose these rather than styling Material widgets
// ad hoc, so the same thing looks and behaves the same everywhere.

/** Corner radius of grouped surfaces (cards, row groups, sheets' inner blocks). */
val GroupShape = RoundedCornerShape(20.dp)

/** The page title at the top of every tab: large, left-aligned, with optional actions on the right. */
@Composable
fun ScreenTitle(title: String, subtitle: String? = null, modifier: Modifier = Modifier, actions: @Composable RowScope.() -> Unit = {}) {
    Row(modifier.fillMaxWidth().padding(top = 12.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(title, Modifier.semantics { heading() }, style = MaterialTheme.typography.headlineMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        actions()
    }
}

/** A small uppercase label introducing a group. */
@Composable
fun SectionHeader(title: String, modifier: Modifier = Modifier) {
    Text(
        title.uppercase(), modifier.padding(start = 4.dp, top = 28.dp, bottom = 10.dp).semantics { heading() },
        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.2.sp,
    )
}

/** A rounded surface holding related rows; rows inside are separated with [RowDivider]. */
@Composable
fun GroupCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(modifier.fillMaxWidth().clip(GroupShape).background(MaterialTheme.colorScheme.surfaceVariant).animateContentSize(Motion.standard()), content = content)
}

/** The inset hairline between rows of a [GroupCard], aligned with the row text. */
@Composable
fun RowDivider(inset: androidx.compose.ui.unit.Dp = 66.dp) =
    HorizontalDivider(Modifier.padding(start = inset), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))

/** The rounded-square tile an [AppRow] icon sits in. */
@Composable
fun IconTile(icon: ImageVector, tint: Color = MaterialTheme.colorScheme.secondary) {
    Box(Modifier.size(36.dp).clip(RoundedCornerShape(10.dp)).background(MaterialTheme.colorScheme.background.copy(alpha = 0.6f)), contentAlignment = Alignment.Center) {
        Icon(icon, null, Modifier.size(20.dp), tint = tint)
    }
}

/**
 * One row of a [GroupCard]: a leading tile or image, a title and optional subtitle, and a trailing
 * control. Tappable rows show a chevron unless given another [trailing].
 */
@Composable
fun AppRow(
    title: String,
    subtitle: String? = null,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    leading: (@Composable () -> Unit)? = icon?.let { { IconTile(it) } },
    titleColor: Color = MaterialTheme.colorScheme.onSurface,
    onClick: (() -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = if (onClick != null) { { Icon(BrandIcons.ChevronRight, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) } } else null,
) {
    Row(
        modifier.fillMaxWidth().heightIn(min = 64.dp)
            .then(if (onClick != null) Modifier.clickable(role = Role.Button, onClick = onClick) else Modifier)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        leading?.invoke()
        Column(Modifier.weight(1f).padding(start = if (leading != null) 14.dp else 0.dp, end = 12.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, color = titleColor, maxLines = 2, overflow = TextOverflow.Ellipsis)
            if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
        trailing?.invoke()
    }
}

/** A text action at the end of an [AppRow] ("Change", "Check"). */
@Composable
fun RowAction(label: String, color: Color = MaterialTheme.colorScheme.secondary) = Text(label, style = MaterialTheme.typography.labelLarge, color = color)

/** The main action of a screen or form: full width, [ControlHeight] tall, sinks on press. */
@Composable
fun PrimaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, loading: Boolean = false, icon: ImageVector? = null) {
    val press = remember { MutableInteractionSource() }
    Button(
        onClick = onClick, enabled = enabled && !loading, interactionSource = press, shape = ControlShape,
        modifier = modifier.fillMaxWidth().height(ControlHeight).pressScale(press, 0.98f),
        colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary, contentColor = MaterialTheme.colorScheme.onPrimary),
    ) { ButtonContent(text, icon, loading, MaterialTheme.colorScheme.onPrimary) }
}

/** A secondary action: the same size as [PrimaryButton], on a quiet tonal fill instead of an outline. */
@Composable
fun SecondaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, loading: Boolean = false, icon: ImageVector? = null, contentColor: Color = MaterialTheme.colorScheme.onSurface) {
    val press = remember { MutableInteractionSource() }
    Button(
        onClick = onClick, enabled = enabled && !loading, interactionSource = press, shape = ControlShape,
        modifier = modifier.height(ControlHeight).pressScale(press, 0.98f),
        colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.surfaceVariant, contentColor = contentColor),
    ) { ButtonContent(text, icon, loading, contentColor) }
}

@Composable
private fun ButtonContent(text: String, icon: ImageVector?, loading: Boolean, color: Color) {
    if (loading) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp, color = color)
    else {
        if (icon != null) Icon(icon, null, Modifier.padding(end = 8.dp).size(18.dp))
        Text(text, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/** A text field in the shared control size and shape, with brand-colored borders. */
@Composable
fun AppTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    placeholder: String? = null,
    supporting: String? = null,
    isError: Boolean = false,
    visualTransformation: VisualTransformation = VisualTransformation.None,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default,
    singleLine: Boolean = true,
    minLines: Int = 1,
) {
    OutlinedTextField(
        value, onValueChange, modifier.fillMaxWidth().heightIn(min = ControlHeight),
        label = { Text(label) }, placeholder = placeholder?.let { { Text(it, maxLines = 1, overflow = TextOverflow.Ellipsis) } },
        supportingText = supporting?.let { { Text(it) } }, isError = isError, singleLine = singleLine, minLines = minLines,
        visualTransformation = visualTransformation, keyboardOptions = keyboardOptions, keyboardActions = keyboardActions, shape = ControlShape,
        colors = OutlinedTextFieldDefaults.colors(
            focusedBorderColor = MaterialTheme.colorScheme.secondary,
            focusedLabelColor = MaterialTheme.colorScheme.secondary,
            cursorColor = MaterialTheme.colorScheme.secondary,
            unfocusedBorderColor = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.35f),
        ),
    )
}

/** What a screen shows when there's nothing in it yet: an icon, what's missing, and what to do. */
@Composable
fun EmptyState(icon: ImageVector, title: String, body: String, modifier: Modifier = Modifier, action: (@Composable () -> Unit)? = null) {
    Column(modifier.fillMaxWidth().padding(vertical = 48.dp, horizontal = 24.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Box(Modifier.size(64.dp).clip(RoundedCornerShape(20.dp)).background(MaterialTheme.colorScheme.surfaceVariant), contentAlignment = Alignment.Center) {
            Icon(icon, null, Modifier.size(28.dp), tint = MaterialTheme.colorScheme.secondary)
        }
        Text(title, style = MaterialTheme.typography.titleLarge, textAlign = TextAlign.Center)
        Text(body, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
        action?.let { Box(Modifier.padding(top = 8.dp)) { it() } }
    }
}
