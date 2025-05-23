// Carga las variables de entorno desde el archivo .env
require('dotenv').config();
const express = require('express');
const { google } = require('googleapis');
const path = require('path');
const nodemailer = require('nodemailer');

const app = express();
const PORT = process.env.PORT || 3000;

app.use(express.json());
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  next();
});

const SPREADSHEET_ID = '1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y';

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
    process.exit(1);
  }
}
initializeGoogleSheetsClient();
const resetCodes = {}; // Almacena los códigos de verificación por ID


function calculateLevel(points) {
  if (points >= 100) return 5;
  if (points >= 75) return 4;
  if (points >= 50) return 3;
  if (points >= 25) return 2;
  if (points >= 10) return 1;
  return 0;
}

app.use(express.static(path.join(__dirname, 'public')));

app.post('/api/login', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { studentId, password } = req.body;

  if (!studentId || !password) {
    return res.status(400).json({ error: 'ID y contraseña requeridos.' });
  }

  try {
    const range = 'Sheet1!A:K';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron alumnos.' });
    }

    const studentRowIndex = rows.findIndex((row, index) => index > 0 && row[0] === studentId && row[4] === password);
    if (studentRowIndex === -1) {
      return res.status(401).json({ error: 'Credenciales inválidas.' });
    }

    const studentData = rows[studentRowIndex];
    const studentName = studentData[1] || 'N/A';
    const tareas = studentData[5] || '0';
    const asistencias = studentData[7] || '0';
    const monedas = studentData[6] || '0';
    const badgg = studentData[8] || '0';
    const puntos = parseInt(studentData[9]) || 0;

    const purchasesResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D'
    });

    const purchaseRows = purchasesResponse.data.values;
    const studentPurchases = purchaseRows?.length > 1
      ? purchaseRows.slice(1).filter(row => row[1] === studentId)
      : [];

    const badgesMap = studentPurchases.reduce((acc, row) => {
      const name = row[2];
      const qty = parseInt(row[3]) || 0;
      acc[name] = (acc[name] || 0) + qty;
      return acc;
    }, {});

    res.json({
      success: true,
      student: {
        id: studentId,
        name: studentName,
        tareas,
        asistencias,
        monedas,
        badgg,
        points: puntos,
        level: calculateLevel(puntos),
        purchases: badgesMap,
        rowIndex: studentRowIndex + 1
      }
    });
  } catch (error) {
    console.error('Error en login:', error);
    res.status(500).json({ error: 'Error en login', details: error.message });
  }
});

app.get('/api/badges', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  try {
    const range = 'Badges!A:C';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length < 2) {
      return res.status(404).json({ error: 'No se encontraron insignias.' });
    }

    const badges = rows.slice(1).map(row => ({
      name: row[0] || 'N/A',
      quantity: parseInt(row[1]) || 0,
      cost: parseInt(row[2]) || 0,
    }));

    res.json({ success: true, badges });
  } catch (error) {
    console.error('Error al obtener insignias:', error);
    res.status(500).json({ error: 'Error al obtener insignias', details: error.message });
  }
});

app.post('/api/purchase', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });

  const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost } = req.body;
  if (!studentId || !studentRowIndex || !itemName || typeof quantityToBuy !== 'number' || typeof itemCost !== 'number') {
    return res.status(400).json({ error: 'Datos inválidos' });
  }

  const totalCost = quantityToBuy * itemCost;

  try {
    const studentCoinsRange = `Sheet1!G${studentRowIndex}`;
    const badgeDataRange = 'Badges!A:C';

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

    const currentQty = parseInt(badgeRows[badgeRowIndex][1]) || 0;
    if (currentQty < quantityToBuy) {
      return res.status(400).json({ error: `Sólo quedan ${currentQty} unidades de ${itemName}` });
    }

    const newCoins = currentCoins - totalCost;
    const newQty = currentQty - quantityToBuy;

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: [
          { range: studentCoinsRange, values: [[newCoins]] },
          { range: `Badges!B${badgeRowIndex + 1}`, values: [[newQty]] }
        ],
        valueInputOption: 'RAW'
      }
    });

    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Purchases!A:D',
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

    const transporter = nodemailer.createTransport({
      service: 'gmail',
      auth: {
        user: process.env.EMAIL_USER,
        pass: process.env.EMAIL_PASS
      }
    });

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: [`${studentId}@up.edu.mx`, 'fcal@up.edu.mx'],
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



app.post('/api/send-reset-code', async (req, res) => {
  const { id } = req.body;
  if (!id) return res.status(400).json({ error: 'ID requerido' });

  try {
    const code = Math.floor(100000 + Math.random() * 900000).toString();
    resetCodes[id] = code;

    const transporter = nodemailer.createTransport({
      service: 'gmail',
      auth: {
        user: process.env.EMAIL_USER,
        pass: process.env.EMAIL_PASS
      }
    });

    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: `${id}@up.edu.mx`,
      subject: 'Código de recuperación de contraseña',
      text: `Tu código para recuperar tu contraseña es: ${code}`
    };

    await transporter.sendMail(mailOptions);
    res.json({ message: 'Código enviado por correo' });
  } catch (error) {
    console.error('Error al enviar código:', error);
    res.status(500).json({ error: 'Error al enviar código', details: error.message });
  }
});

app.post('/api/reset-password-with-code', async (req, res) => {
  const { id, code, password } = req.body;
  console.log({ id, code, realCode: resetCodes[id] });

  if (!id || !code || !password || !/^[A-Z0-9]{4}$/.test(password)) {
    return res.status(400).json({ error: 'Datos inválidos. Asegúrate de usar un código y contraseña válidos.' });
  }

  try {
    if (!resetCodes[id] || resetCodes[id] !== code) {
      return res.status(401).json({ error: 'Código incorrecto o no solicitado.' });
    }

    const range = 'Sheet1!A:E';
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    const index = rows.findIndex((row, i) => i > 0 && row[0] === id);
    if (index === -1) {
      return res.status(404).json({ error: 'ID no encontrado.' });
    }

    const rowNumber = index + 1;
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Sheet1!E${rowNumber}`,
      valueInputOption: 'USER_ENTERED',
      requestBody: { values: [[password]] }
    });

    delete resetCodes[id]; // invalidar el código usado

    res.json({ message: 'Contraseña actualizada correctamente.' });
  } catch (error) {
    console.error('Error al actualizar contraseña con código:', error);
    res.status(500).json({ error: 'Error al actualizar la contraseña', details: error.message });
  }
});


app.listen(PORT, () => {
  console.log(`Servidor backend escuchando en http://localhost:${PORT}`);
});
