package dev.temper.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.temper.ui.theme.Action
import dev.temper.ui.theme.Card
import dev.temper.ui.theme.DataFont
import dev.temper.ui.theme.Hair
import dev.temper.ui.theme.HairStrong
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Ivory
import dev.temper.ui.theme.Lift
import dev.temper.ui.theme.OnNavigation
import dev.temper.ui.theme.Panel
import dev.temper.ui.theme.PaperSoft
import dev.temper.ui.theme.Plum

@Composable
fun TemperTopBar(
    title: String,
    subtitle: String? = null,
    onBack: (() -> Unit)? = null,
    trailing: @Composable (() -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(Plum)
            .statusBarsPadding()
            .heightIn(min = 64.dp)
            .padding(start = if (onBack == null) 16.dp else 4.dp, end = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            Box(
                modifier = Modifier
                    .size(48.dp)
                    .clickable(onClick = onBack),
                contentAlignment = Alignment.Center,
            ) {
                Text("←", color = OnNavigation, fontSize = 24.sp)
            }
        }
        Column(modifier = Modifier.weight(1f).padding(start = if (onBack == null) 0.dp else 4.dp)) {
            Text(
                text = title,
                color = OnNavigation,
                style = MaterialTheme.typography.titleLarge,
                maxLines = 1,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    color = OnNavigation.copy(alpha = 0.82f),
                    style = MaterialTheme.typography.labelMedium,
                    maxLines = 1,
                )
            }
        }
        trailing?.invoke()
    }
}

@Composable
fun TemperScaffold(
    topBar: @Composable () -> Unit,
    rail: @Composable (ColumnScope.() -> Unit)? = null,
    content: @Composable BoxScope.() -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(Ivory),
    ) {
        topBar()
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            content()
        }
        if (rail != null) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(Ivory)
                    .border(BorderStroke(1.dp, Hair))
                    .navigationBarsPadding()
                    .padding(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 20.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                content = rail,
            )
        }
    }
}

@Composable
fun PrimaryButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth().heightIn(min = 64.dp),
        shape = RoundedCornerShape(2.dp),
        colors = ButtonDefaults.buttonColors(
            containerColor = Action,
            contentColor = Ink,
            disabledContainerColor = PaperSoft,
            disabledContentColor = Panel,
        ),
    ) {
        Text(text = text, style = MaterialTheme.typography.labelLarge, textAlign = TextAlign.Center)
    }
}

@Composable
fun QuietButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth().heightIn(min = 56.dp),
        shape = RoundedCornerShape(2.dp),
        border = BorderStroke(1.dp, if (enabled) Ink else HairStrong),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = Ink),
    ) {
        Text(text = text, style = MaterialTheme.typography.titleMedium, textAlign = TextAlign.Center)
    }
}

@Composable
fun SmallTextButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.heightIn(min = 48.dp),
        shape = RoundedCornerShape(2.dp),
        border = BorderStroke(1.dp, HairStrong),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = Ink),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp, vertical = 10.dp),
    ) {
        Text(text = text, style = MaterialTheme.typography.labelMedium.copy(fontFamily = FontFamily.Monospace))
    }
}

@Composable
fun TopBarAction(text: String, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .heightIn(min = 48.dp)
            .clickable(onClick = onClick)
            .padding(horizontal = 12.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = text.uppercase(),
            color = OnNavigation,
            style = MaterialTheme.typography.labelMedium,
        )
    }
}

@Composable
fun RailHint(left: String, right: String? = null) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(
            text = left,
            color = Panel,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.weight(1f),
        )
        if (right != null) {
            Spacer(modifier = Modifier.width(12.dp))
            Text(text = right, color = Panel, style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
fun Eyebrow(text: String, color: Color = Panel) {
    Text(
        text = text.uppercase(),
        color = color,
        style = MaterialTheme.typography.labelMedium,
        fontWeight = FontWeight.SemiBold,
    )
}

@Composable
fun Hairline(modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxWidth().height(1.dp).background(Hair))
}

@Composable
fun AccentBlock(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .shadow(3.dp, shape = RoundedCornerShape(0.dp), ambientColor = Lift, spotColor = Lift)
            .background(Card)
            .drawBehind {
                drawRect(color = Action, size = Size(size.width, 7.dp.toPx()))
                drawRect(color = Action, size = Size(6.dp.toPx(), size.height))
            }
            .padding(start = 18.dp, end = 14.dp, top = 17.dp, bottom = 14.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
        content = content,
    )
}

@Composable
fun SyncStrip(
    mark: String,
    title: String,
    detail: String,
    dark: Boolean = false,
    dashed: Boolean = false,
) {
    val background = if (dark) Plum else PaperSoft
    val foreground = if (dark) OnNavigation else Ink
    val secondary = if (dark) OnNavigation.copy(alpha = 0.8f) else Panel
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(background)
            .drawBehind { drawRect(color = if (dark) Action else Ink, size = Size(6.dp.toPx(), size.height)) }
            .padding(start = 16.dp, end = 14.dp, top = 12.dp, bottom = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = mark,
            color = foreground,
            fontFamily = DataFont,
            fontWeight = FontWeight.SemiBold,
            fontSize = 17.sp,
            modifier = Modifier.widthIn(min = 18.dp),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(text = title, color = foreground, style = MaterialTheme.typography.titleMedium)
            Text(text = detail, color = secondary, style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
fun LoadingPane() {
    Column(
        modifier = Modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(1.dp),
    ) {
        Box(modifier = Modifier.fillMaxWidth().height(14.dp).background(PaperSoft))
        repeat(4) {
            Box(modifier = Modifier.fillMaxWidth().height(56.dp).background(PaperSoft))
        }
        Spacer(modifier = Modifier.height(8.dp))
        Text(
            text = "Reading from the server. Nothing is cached on this phone.",
            color = Panel,
            style = MaterialTheme.typography.labelMedium,
        )
    }
}

@Composable
fun ErrorPane(
    title: String,
    detail: String,
    action: String? = null,
    onAction: (() -> Unit)? = null,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SyncStrip(mark = "✗", title = title, detail = detail)
        if (action != null && onAction != null) {
            QuietButton(text = action, onClick = onAction)
        }
    }
}

@Composable
fun EmptyPane(
    eyebrow: String,
    title: String,
    detail: String,
    action: String? = null,
    onAction: (() -> Unit)? = null,
) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        AccentBlock {
            Eyebrow(eyebrow)
            Text(text = title, color = Ink, style = MaterialTheme.typography.headlineLarge)
            Text(text = detail, color = Ink, style = MaterialTheme.typography.bodyLarge)
        }
        if (action != null && onAction != null) {
            QuietButton(text = action, onClick = onAction)
        }
    }
}

@Composable
fun TemperTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    singleLine: Boolean = true,
    isError: Boolean = false,
    supportingText: String? = null,
    visualTransformation: VisualTransformation = VisualTransformation.None,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    minHeight: Dp = 64.dp,
) {
    Column(modifier = modifier) {
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            label = { Text(label, style = MaterialTheme.typography.labelMedium) },
            enabled = enabled,
            singleLine = singleLine,
            isError = isError,
            visualTransformation = visualTransformation,
            keyboardOptions = keyboardOptions,
            textStyle = MaterialTheme.typography.bodyLarge,
            shape = RoundedCornerShape(2.dp),
            modifier = Modifier.fillMaxWidth().heightIn(min = minHeight),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Ink,
                unfocusedBorderColor = HairStrong,
                focusedContainerColor = Card,
                unfocusedContainerColor = Card,
                disabledContainerColor = PaperSoft,
                errorBorderColor = Ink,
                cursorColor = Ink,
                focusedTextColor = Ink,
                unfocusedTextColor = Ink,
            ),
        )
        if (supportingText != null) {
            Text(
                text = supportingText,
                color = if (isError) Ink else Panel,
                style = MaterialTheme.typography.labelMedium,
                modifier = Modifier.padding(start = 4.dp, top = 4.dp),
            )
        }
    }
}

@Composable
fun MonoText(
    text: String,
    modifier: Modifier = Modifier,
    color: Color = Ink,
    size: Int = 14,
    weight: FontWeight = FontWeight.Normal,
) {
    Text(
        text = text,
        modifier = modifier,
        color = color,
        fontFamily = DataFont,
        fontWeight = weight,
        fontSize = size.sp,
    )
}
