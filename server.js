// Carga las variables de entorno desde el archivo .env
require('dotenv').config();
const express = require('express');
const { google } = require('googleapis');
const path = require('path');
const nodemailer = require('nodemailer');
// const bcrypt = require('bcryptjs'); // Descomentar si decides usar hashing de contraseñas

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware para JSON y CORS
app.use(express.json());
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  next();
});

// Sirve archivos estáticos (HTML, CSS, JS, imágenes) desde la carpeta 'public'
app.use(express.static(path.join(__dirname, 'public')));

// Configuración de Google Sheets
const SPREADSHEET_ID = '1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y'; // Tu ID de Spreadsheet

const serviceAccount = JSON.parse(process.env.GOOGLE_SERVICE_ACCOUNT);
const auth = new google.auth.GoogleAuth({
  credentials: serviceAccount,
  scopes: ['https://www.googleapis.com/auth/spreadsheets']
});

let sheets;
async function initializeGoogleSheetsClient() {
  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });
    console.log('Cliente de Google Sheets inicializado correctamente.');
  } catch (error) {
    console.error('Error al inicializar Google Sheets:', error);
    process.exit(1); // Sale del proceso si no se puede inicializar el cliente
  }
}
initializeGoogleSheetsClient();

// Almacenamiento temporal para códigos de restablecimiento de contraseña
const resetCodes = {};

// Función para calcular el nivel basada en la EXP.
function calculateLevel(exp) {
  if (exp >= 100) return 5;
  if (exp >= 75) return 4;
  if (exp >= 50) return 3;
  if (exp >= 25) return 2;
  if (exp >= 10) return 1;
  return 0;
}

// Configuración de Nodemailer para el envío de correos
const transporter = nodemailer.createTransport({
  service: 'gmail', // Puedes usar otro servicio o configuración SMTP
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASS,
  },
});

// ===================================
// ENDPOINTS DE API
// ===================================

// Endpoint de Inicio de Sesión
app.post('/api/login', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { studentId, password } = req.body;

  if (!studentId || !password) {
    return res.status(400).json({ error: 'ID y contraseña requeridos.' });
  }

  try {
    // Rango ajustado para incluir hasta la nueva columna K (LAST_LOGIN_LEVEL)
    const range = 'Sheet1!A:K'; // Ahora leemos hasta la columna K
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron datos de alumnos.' });
    }

    // Buscar el alumno por ID y Contraseña. La contraseña está en Columna E (índice 4)
    const studentRowIndex = rows.findIndex((row, index) => index > 0 && row[0] === studentId && row[4] === password);
    if (studentRowIndex === -1) {
      return res.status(401).json({ error: 'Credenciales inválidas.' });
    }

    const studentData = rows[studentRowIndex]; // Datos de la fila encontrada
    const id = studentData[0]; // ID
    const name = studentData[1] || 'N/A'; // NOMBRE
    const sexo = studentData[2] || 'N/A'; // Sexo
    const email = studentData[3] || ''; // CORREO
    const homeworks = studentData[5] || '0'; // Homeworks
    const monedas = studentData[6] || '0'; // Coins
    const asistencias = studentData[7] || '0'; // Attendance
    const exp = parseInt(studentData[9]) || 0; // EXP

    // Obtener lastLoginLevel. Si está vacío o es undefined (primera carga), se inicializa a 0
    let lastLoginLevel = parseInt(studentData[10]);
    if (isNaN(lastLoginLevel)) { // Si no es un número (es decir, está vacío en la hoja)
        lastLoginLevel = 0; // Considera el nivel en el último login como 0 inicialmente
    }

    const currentLevel = calculateLevel(exp); // Nivel actual calculado

    // Determinar si hay una subida de nivel desde el último login
    const levelUpOccurred = currentLevel > lastLoginLevel;

    // Obtener las compras de la hoja 'Purchases'
    const purchasesResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D'
    });

    const purchaseRows = purchasesResponse.data.values;
    // Filtrar las compras para este studentId
    const studentPurchases = purchaseRows?.length > 1
      ? purchaseRows.slice(1).filter(row => row[1] === id) // row[1] es studentId en Purchases
      : [];

    // Reducir las compras a un mapa de insignias y cantidades
    const badgesMap = studentPurchases.reduce((acc, row) => {
      const badgeName = row[2]; // Columna C es el nombre de la insignia en Purchases
      const quantity = parseInt(row[3]) || 0; // Columna D es la cantidad en Purchases
      acc[badgeName] = (acc[badgeName] || 0) + quantity;
      return acc;
    }, {});

    res.json({
      success: true,
      student: {
        id: id,
        name: name,
        sexo: sexo,
        email: email,
        monedas: monedas,
        tareas: homeworks,
        asistencias: asistencias,
        exp: exp,
        level: currentLevel, // Nivel actual
        purchases: badgesMap,
        rowIndex: studentRowIndex + 1 // Fila en el Sheets (para futuras actualizaciones)
      },
      // Indicador para el frontend
      levelUpOccurred: levelUpOccurred
    });

    // ¡Importante! Actualizar el LAST_LOGIN_LEVEL en Google Sheets después del login exitoso
    // Esto asegura que la animación no se dispare en el próximo inicio de sesión si no hay cambio de nivel real.
    // Y también inicializa el valor si estaba vacío.
    if (currentLevel !== lastLoginLevel || isNaN(parseInt(studentData[10]))) { // Actualiza si hay cambio de nivel O si estaba vacío
      await sheets.spreadsheets.values.update({
        spreadsheetId: SPREADSHEET_ID,
        range: `Sheet1!K${studentRowIndex + 1}`, // Columna K para LAST_LOGIN_LEVEL
        valueInputOption: 'RAW',
        requestBody: { values: [[currentLevel]] }
      });
    }

  } catch (error) {
    console.error('Error en el login:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el login.', details: error.message });
  }
});

// Endpoint de Registro
app.post('/api/register', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { id, name, sexo, password } = req.body;
  if (!id || !name || !sexo || !password || !/^[A-Z0-9]{4}$/.test(password)) {
    return res.status(400).json({ error: 'Datos inválidos o incompletos. Contraseña debe ser 4 HEX mayúsculas.' });
  }

  try {
    const range = 'Sheet1!A:A';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const existingIds = response.data.values ? response.data.values.flat() : [];

    if (existingIds.includes(id)) {
      return res.status(409).json({ error: 'El ID de alumno ya existe.' });
    }

    const storedPassword = password; // Almacenando como texto plano

    const newRow = [
      id,                  // Columna A: ID
      name,                // Columna B: NOMBRE
      sexo,                // Columna C: Sexo
      '',                  // Columna D: CORREO (vacío por defecto)
      storedPassword,      // Columna E: PAS (Contraseña)
      '0',                 // Columna F: Homeworks (0 por defecto)
      '0',                 // Columna G: Coins (0 por defecto)
      '0',                 // Columna H: Attendance (0 por defecto)
      '0',                 // Columna I: Badges (inicializado con '0')
      '0',                 // Columna J: EXP (0 por defecto)
      '0'                  // Columna K: LAST_LOGIN_LEVEL (0 por defecto para nuevos usuarios)
    ];

    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A1', // Se agrega al final de la hoja
      valueInputOption: 'RAW',
      requestBody: { values: [newRow] }
    });

    res.status(201).json({ message: 'Alumno registrado exitosamente.' });
  } catch (error) {
    console.error('Error en el registro:', error);
    res.status(500).json({ error: 'Error interno del servidor durante el registro.', details: error.message });
  }
});


// Endpoint para enviar código de restablecimiento
app.post('/api/send-reset-code', async (req, res) => {
  const { id } = req.body;
  if (!id) {
    return res.status(400).json({ error: 'ID de alumno es requerido.' });
  }

  try {
    // Rango para obtener ID y Email (Columna D)
    const range = 'Sheet1!A:D';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const studentRow = rows.find((row, i) => i > 0 && row[0] === id); // Buscar por ID
    if (!studentRow) {
      return res.status(404).json({ error: 'ID de alumno no encontrado.' });
    }

    const studentEmail = studentRow[3]; // Columna D para el correo electrónico
    if (!studentEmail || studentEmail.trim() === '') {
      return res.status(400).json({ error: 'Este alumno no tiene un correo electrónico registrado para restablecer la contraseña.' });
    }

    const code = Math.floor(100000 + Math.random() * 900000).toString(); // Código de 6 dígitos
    resetCodes[id] = code; // Almacena el código temporalmente

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: [`${studentId}@up.edu.mx`, 'fcal@up.edu.mx'], // Ajusta los destinatarios según tu necesidad
      subject: 'Código de restablecimiento de contraseña',
      text: `Tu código de restablecimiento de contraseña es: ${code}\nEste código expirará en 10 minutos.`
    };

    await transporter.sendMail(mailOptions);
    console.log(`Código de restablecimiento enviado a ${studentEmail} para ID ${id}`);

    // Limpia el código después de 10 minutos
    setTimeout(() => {
      delete resetCodes[id];
      console.log(`Código de restablecimiento para ID ${id} expirado.`);
    }, 10 * 60 * 1000); // 10 minutos

    res.json({ message: 'Código de restablecimiento enviado a tu correo electrónico.' });
  } catch (error) {
    console.error('Error al enviar el código de restablecimiento:', error);
    res.status(500).json({ error: 'Error al enviar el código de restablecimiento.', details: error.message });
  }
});

// Endpoint para restablecer contraseña con código
app.post('/api/reset-password-with-code', async (req, res) => {
  const { id, code, password } = req.body;
  console.log({ id, code, realCode: resetCodes[id] });

  if (!id || !code || !password || !/^[A-Z0-9]{4}$/.test(password)) {
    return res.status(400).json({ error: 'Datos inválidos. Asegúrate de usar un código y contraseña válidos (4 HEX mayúsculas).' });
  }

  try {
    if (!resetCodes[id] || resetCodes[id] !== code) {
      return res.status(401).json({ error: 'Código incorrecto o no solicitado.' });
    }

    const range = 'Sheet1!A:A'; // Solo necesitamos la columna A para encontrar la fila
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === id); // Buscar por ID
    if (index === -1) {
      return res.status(404).json({ error: 'ID no encontrado.' });
    }

    const rowNumber = index + 1; // Ajustar a número de fila de Sheets
    const newPassword = password; // Almacenando como texto plano

    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Sheet1!E${rowNumber}`, // Columna E para la contraseña (PAS)
      valueInputOption: 'RAW',
      requestBody: { values: [[newPassword]] }
    });

    delete resetCodes[id]; // Eliminar el código después de usarlo
    res.json({ message: 'Contraseña actualizada correctamente.' });
  } catch (error) {
    console.error('Error al actualizar contraseña:', error);
    res.status(500).json({ error: 'Error al actualizar contraseña.', details: error.message });
  }
});

// Endpoint para obtener insignias
app.get('/api/badges', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  try {
    const range = 'Badges!A:C'; // Suponiendo Insignia, Cantidad, Costo
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length < 2) { // Asume que la primera fila es el encabezado
      return res.status(404).json({ error: 'No se encontraron insignias o formato incorrecto.' });
    }

    const badges = rows.slice(1).map(row => ({ // Ignorar encabezado
      name: row[0],     // Columna A: Nombre de la Insignia
      quantity: parseInt(row[1]) || 0, // Columna B: Cantidad disponible
      cost: parseInt(row[2]) || 0      // Columna C: Costo
    }));

    res.json({ success: true, badges });
  } catch (error) {
    console.error('Error al obtener insignias:', error);
    res.status(500).json({ error: 'Error al obtener insignias', details: error.message });
  }
});

// Endpoint para realizar una compra
app.post('/api/purchase', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost } = req.body;
  if (!studentId || !studentRowIndex || !itemName || typeof quantityToBuy !== 'number' || typeof itemCost !== 'number') {
    return res.status(400).json({ error: 'Datos inválidos' });
  }

  const totalCost = quantityToBuy * itemCost;

  try {
    const studentCoinsRange = `Sheet1!G${studentRowIndex}`; // Columna G: Coins
    const badgeDataRange = 'Badges!A:C'; // Nombre, Cantidad, Costo de insignias

    const [coinsResponse, badgesResponse] = await Promise.all([
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: studentCoinsRange }),
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: badgeDataRange })
    ]);

    const currentCoins = parseInt(coinsResponse.data.values?.[0]?.[0]) || 0;
    if (currentCoins < totalCost) {
      return res.status(400).json({ error: 'Monedas insuficientes' });
    }

    const badgeRows = badgesResponse.data.values;
    const badgeRowIndex = badgeRows.findIndex((row, i) => i > 0 && row[0] === itemName);
    if (badgeRowIndex === -1) return res.status(404).json({ error: 'Insignia no encontrada' });

    const currentQty = parseInt(badgeRows[badgeRowIndex][1]) || 0; // Cantidad disponible de la insignia
    if (currentQty < quantityToBuy) {
      return res.status(400).json({ error: `Sólo quedan ${currentQty} unidades de ${itemName}` });
    }

    const newCoins = currentCoins - totalCost;
    const newQty = currentQty - quantityToBuy;

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: [
          { range: studentCoinsRange, values: [[newCoins]] }, // Columna G: Monedas
          { range: `Badges!B${badgeRowIndex + 1}`, values: [[newQty]] } // Columna B de la hoja Badges
        ],
        valueInputOption: 'RAW' // RAW para valores directos
      }
    });

    // Registrar la compra en la hoja 'Purchases'
    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D', // Asegúrate de que esta hoja y columnas existan
      valueInputOption: 'USER_ENTERED',
      requestBody: {
        values: [[
          new Date().toLocaleString(),
          studentId,
          itemName,
          quantityToBuy
        ]]
      }
    });

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: [`${studentId}@up.edu.mx`, 'fcal@up.edu.mx'], // Ajusta los destinatarios según tu necesidad
      subject: 'Confirmación de compra de insignia',
      text: `Hola,

Se ha realizado una compra de insignia:

- Alumno: ${studentId}@up.edu.mx
- Insignia: ${itemName}
- Cantidad: ${quantityToBuy}
- Costo total: ${totalCost} monedas
- Monedas anteriores: ${currentCoins}
- Monedas restantes: ${newCoins}

Gracias.`
    };

    await transporter.sendMail(mailOptions);

    res.json({
      success: true,
      newCoins,
      newBadgeQuantity: newQty,
      message: `Compra de ${quantityToBuy} ${itemName} realizada. Monedas restantes: ${newCoins}`
    });
  } catch (error) {
    console.error('Error en la compra:', error);
    res.status(500).json({ error: 'Error en la compra', details: error.message });
  }
});

// Endpoint para añadir EXP.
app.post('/api/add-exp', async (req, res) => {
  const { studentId, expToAdd } = req.body;

  if (!studentId || expToAdd === undefined || isNaN(expToAdd) || expToAdd <= 0) {
    return res.status(400).json({ error: 'ID de alumno y cantidad de EXP. válida son requeridos.' });
  }

  try {
    const range = 'Sheet1!A:K'; // Necesitamos ID, EXP. (J) y LAST_LOGIN_LEVEL (K)
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === studentId); // Buscar por ID
    if (index === -1) {
      return res.status(404).json({ error: 'ID de alumno no encontrado.' });
    }

    const rowNumber = index + 1;
    const currentExp = parseInt(rows[index][9]) || 0; // Columna J (índice 9) es EXP.
    let currentLastLoginLevel = parseInt(rows[index][10]); // Columna K (índice 10) es LAST_LOGIN_LEVEL

    if (isNaN(currentLastLoginLevel)) { // Si está vacío, inicialízalo a 0
        currentLastLoginLevel = 0;
    }

    const newExp = currentExp + expToAdd;
    const newLevel = calculateLevel(newExp);

    const updates = [{
      range: `Sheet1!J${rowNumber}`, // Columna J: EXP.
      values: [[newExp]]
    }];

    // Actualizar LAST_LOGIN_LEVEL solo si el nuevo nivel es mayor y no ha sido reflejado
    // O si estaba vacío.
    if (newLevel > currentLastLoginLevel || isNaN(parseInt(rows[index][10]))) {
        updates.push({
            range: `Sheet1!K${rowNumber}`, // Columna K: LAST_LOGIN_LEVEL
            values: [[newLevel]]
        });
    }

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: updates,
        valueInputOption: 'RAW'
      }
    });

    res.json({
      success: true,
      message: `EXP. de ${studentId} actualizada. Nueva EXP.: ${newExp}, Nuevo Nivel: ${newLevel}.`,
      newExp: newExp,
      newLevel: newLevel
    });

  } catch (error) {
    console.error('Error al añadir EXP.:', error);
    res.status(500).json({ error: 'Error interno del servidor al añadir EXP.', details: error.message });
  }
});


// ----------------- INICIAR SERVIDOR -----------------
app.listen(PORT, () => {
  console.log(`Servidor backend escuchando en http://localhost:${PORT}`);
});